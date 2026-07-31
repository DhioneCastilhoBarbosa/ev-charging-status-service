package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"ev-charging-status-service/internal/repository"
	"ev-charging-status-service/internal/service"
)

type WSHandler struct {
	apiKey      string
	auth        *service.WSAuthService
	usersRepo   *repository.UserRepository
	credsRepo   *repository.CredentialsRepository
	hub         *WSHub
	onConnected func(ctx context.Context, userID string)
	upgrader    websocket.Upgrader
}

func NewWSHandler(
	apiKey string,
	auth *service.WSAuthService,
	usersRepo *repository.UserRepository,
	credsRepo *repository.CredentialsRepository,
	hub *WSHub,
	onConnected func(ctx context.Context, userID string),
) *WSHandler {
	return &WSHandler{
		apiKey:      apiKey,
		auth:        auth,
		usersRepo:   usersRepo,
		credsRepo:   credsRepo,
		hub:         hub,
		onConnected: onConnected,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *WSHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/ws/token", h.handleToken)
	rg.GET("/ws", h.handleConnect)
	rg.GET("/ws/stats", h.handleStats)
}

// handleToken emite JWT de sessão (sem expiresIn; validade = idle + existência do usuário).
func (h *WSHandler) handleToken(c *gin.Context) {
	if h.apiKey != "" && c.GetHeader("X-API-Key") != h.apiKey {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	username := strings.TrimSpace(c.Query("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}
	user, err := h.usersRepo.GetByUsername(c.Request.Context(), username)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	hasCreds, err := h.credsRepo.ExistsByUserID(c.Request.Context(), user.ID)
	if err != nil || !hasCreds {
		c.JSON(http.StatusNotFound, gin.H{"error": "user has no active credentials"})
		return
	}
	token, err := h.auth.GenerateToken(c.Request.Context(), user.ID, user.Username)
	if err != nil {
		log.Printf("[ws] token generation failed for user=%s: %v", user.Username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ws token unavailable"})
		return
	}
	c.JSON(http.StatusOK, ConfigResponse{Token: token})
}

// handleConnect faz upgrade para WebSocket.
func (h *WSHandler) handleConnect(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if len(authHeader) > 7 && strings.EqualFold(authHeader[:7], "bearer ") {
			token = strings.TrimSpace(authHeader[7:])
		}
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing ws token"})
		return
	}
	claims, err := h.auth.ValidateToken(c.Request.Context(), token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid ws token"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[ws] upgrade failed userId=%s: %v", claims.UserID, err)
		return
	}

	idle := h.auth.IdleTimeout()
	client := &wsClient{
		connectionID:  newConnectionID(),
		userID:        claims.UserID,
		conn:          conn,
		send:          make(chan wsMessage, 32),
		hub:           h.hub,
		idleTimeout:   idle,
		onAppActivity: h.touchActivity,
		onIdleTimeout: h.invalidateOnIdle,
	}
	h.hub.Register(client)
	// Conexão conta como início de atividade de sessão nesta conexão.
	client.markAppActivity()
	log.Printf("[ws] connected connectionId=%s userId=%s activeUserConnections=%d idle=%s",
		client.connectionID, client.userID, h.hub.ActiveConnections(client.userID), idle)

	if h.onConnected != nil {
		uid := client.userID
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.onConnected(ctx, uid)
		}()
	}

	go client.writePump()
	client.readPump()
}

func (h *WSHandler) touchActivity(userID string) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := h.auth.TouchActivity(ctx, uid); err != nil {
		log.Printf("[ws] touch activity failed userId=%s: %v", userID, err)
	}
}

func (h *WSHandler) invalidateOnIdle(userID string) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Se outra conexão do mesmo user ainda teve tráfego de app, não invalida a sessão.
	last, ok, err := h.usersRepo.GetWSLastActivity(ctx, uid)
	if err != nil {
		log.Printf("[ws] idle recheck activity failed userId=%s: %v", userID, err)
		return
	}
	if ok && time.Since(last) <= h.auth.IdleTimeout() {
		log.Printf("[ws] idle close without session invalidate (activity fresh) userId=%s", userID)
		return
	}
	if err := h.auth.InvalidateSession(ctx, uid); err != nil {
		log.Printf("[ws] invalidate session on idle failed userId=%s: %v", userID, err)
		return
	}
	h.hub.CloseUserConnections(userID)
	log.Printf("[ws] session invalidated by idle timeout userId=%s", userID)
}

// handleStats retorna métricas do hub.
func (h *WSHandler) handleStats(c *gin.Context) {
	if h.apiKey != "" && c.GetHeader("X-API-Key") != h.apiKey {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, h.hub.Stats())
}

func (c *wsClient) readPump() {
	defer func() {
		c.hub.Unregister(c)
		_ = c.conn.Close()
		log.Printf("[ws] disconnected connectionId=%s userId=%s activeUserConnections=%d", c.connectionID, c.userID, c.hub.ActiveConnections(c.userID))
	}()
	_ = c.conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		// Ping/pong NÃO renovam idle da sessão — só o keepalive do socket (~70s).
		_ = c.conn.SetReadDeadline(time.Now().Add(70 * time.Second))
		return nil
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
		// Mensagem de aplicação do cliente conta como tráfego.
		c.markAppActivity()
	}
}

func (c *wsClient) writePump() {
	pingTicker := time.NewTicker(25 * time.Second)
	idleCheck := time.NewTicker(15 * time.Second)
	defer func() {
		pingTicker.Stop()
		idleCheck.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(msg.messageType, msg.payload); err != nil {
				c.hub.IncrementWriteError()
				return
			}
			// Frame de aplicação (JSON de estações) conta como tráfego — não ping.
			if msg.messageType == websocket.TextMessage || msg.messageType == websocket.BinaryMessage {
				c.markAppActivity()
			}
		case <-pingTicker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
				c.hub.IncrementWriteError()
				return
			}
		case <-idleCheck.C:
			if c.idleTimeout <= 0 {
				continue
			}
			last := c.lastActivityTime()
			if last.IsZero() {
				continue
			}
			if time.Since(last) <= c.idleTimeout {
				continue
			}
			log.Printf("[ws] idle timeout connectionId=%s userId=%s since=%s", c.connectionID, c.userID, time.Since(last))
			if c.onIdleTimeout != nil {
				c.onIdleTimeout(c.userID)
			}
			_ = c.conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "idle timeout"),
				time.Now().Add(2*time.Second),
			)
			return
		}
	}
}

func newConnectionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	return hex.EncodeToString(b)
}
