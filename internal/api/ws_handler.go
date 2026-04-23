package api

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"ev-charging-status-service/internal/repository"
	"ev-charging-status-service/internal/service"
)

type WSHandler struct {
	apiKey    string
	tokenTTL  int
	auth      *service.WSAuthService
	usersRepo *repository.UserRepository
	credsRepo *repository.CredentialsRepository
	hub       *WSHub
	upgrader  websocket.Upgrader
}

func NewWSHandler(
	apiKey string,
	tokenTTL int,
	auth *service.WSAuthService,
	usersRepo *repository.UserRepository,
	credsRepo *repository.CredentialsRepository,
	hub *WSHub,
) *WSHandler {
	return &WSHandler{
		apiKey:    apiKey,
		tokenTTL:  tokenTTL,
		auth:      auth,
		usersRepo: usersRepo,
		credsRepo: credsRepo,
		hub:       hub,
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

// handleToken emite JWT curto para handshake do WebSocket (alternativa ao token retornado em POST /v1/config).
//
//	@Summary		Token WebSocket
//	@Description	Retorna `token` e `expiresIn` (segundos) para usar em `GET /v1/ws?token=` ou `ws://.../v1/ws?token=`.
//	@Tags			WebSocket
//	@Produce		json
//	@Param			username	query		string	true	"Email do usuário (username em `users`, igual ao POST /v1/config)"
//	@Success		200			{object}	ConfigResponse	"token, expiresIn"
//	@Failure		400			{object}	ErrorResponse	"username is required"
//	@Failure		401			{object}	ErrorResponse	"unauthorized"
//	@Failure		404			{object}	ErrorResponse	"user not found / sem credenciais"
//	@Failure		500			{object}	ErrorResponse	"ws token unavailable"
//	@Security		ApiKeyAuth
//	@Router			/v1/ws/token [get]
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
	token, err := h.auth.GenerateToken(user.ID, user.Username)
	if err != nil {
		log.Printf("[ws] token generation failed for user=%s: %v", user.Username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ws token unavailable"})
		return
	}
	c.JSON(http.StatusOK, ConfigResponse{
		Token:     token,
		ExpiresIn: h.tokenTTL,
	})
}

// handleConnect faz upgrade para WebSocket; só entrega mensagens ao userId do JWT.
//
//	@Summary		Conexão WebSocket
//	@Description	Upgrade HTTP para WebSocket. Envie o JWT em `?token=` ou no header `Authorization: Bearer`. Mensagens são JSON (`userId`, `stations`, `timestamp`) no intervalo configurado em `WS_PUBLISH_INTERVAL_SECONDS`.
//	@Tags			WebSocket
//	@Param			token	query		string	false	"JWT retornado por POST /v1/config ou GET /v1/ws/token"
//	@Failure		401		{object}	ErrorResponse	"missing ou invalid ws token"
//	@Router			/v1/ws [get]
func (h *WSHandler) handleConnect(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		if authHeader := c.GetHeader("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		}
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing ws token"})
		return
	}
	claims, err := h.auth.ValidateToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid ws token"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[ws] upgrade failed userId=%s: %v", claims.UserID, err)
		return
	}

	client := &wsClient{
		connectionID: newConnectionID(),
		userID:       claims.UserID,
		conn:         conn,
		send:         make(chan wsMessage, 32),
		hub:          h.hub,
	}
	h.hub.Register(client)
	log.Printf("[ws] connected connectionId=%s userId=%s activeUserConnections=%d", client.connectionID, client.userID, h.hub.ActiveConnections(client.userID))

	go client.writePump()
	client.readPump()
}

// handleStats retorna métricas do hub WebSocket (operacional).
//
//	@Summary		Métricas do hub WebSocket
//	@Description	Conexões ativas, mensagens enviadas, drops por backpressure e erros de escrita.
//	@Tags			WebSocket
//	@Produce		json
//	@Success		200	{object}	WSHubStats
//	@Failure		401	{object}	ErrorResponse	"unauthorized"
//	@Security		ApiKeyAuth
//	@Router			/v1/ws/stats [get]
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
		_ = c.conn.SetReadDeadline(time.Now().Add(70 * time.Second))
		return nil
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *wsClient) writePump() {
	ticker := time.NewTicker(25 * time.Second)
	defer func() {
		ticker.Stop()
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
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
				c.hub.IncrementWriteError()
				return
			}
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
