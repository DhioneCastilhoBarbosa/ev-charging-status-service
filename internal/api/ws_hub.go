package api

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type wsMessage struct {
	messageType int
	payload     []byte
}

type wsClient struct {
	connectionID string
	userID       string
	conn         *websocket.Conn
	send         chan wsMessage
	hub          *WSHub
}

type WSHub struct {
	mu                 sync.RWMutex
	clientsByUser      map[string]map[*wsClient]struct{}
	totalConnections   int64
	droppedByBackpress int64
	messagesSent       int64
	writeErrors        int64
}

func NewWSHub() *WSHub {
	return &WSHub{
		clientsByUser: make(map[string]map[*wsClient]struct{}),
	}
}

func (h *WSHub) Register(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clientsByUser[c.userID]; !ok {
		h.clientsByUser[c.userID] = make(map[*wsClient]struct{})
	}
	h.clientsByUser[c.userID][c] = struct{}{}
	h.totalConnections++
}

func (h *WSHub) Unregister(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients, ok := h.clientsByUser[c.userID]
	if !ok {
		return
	}
	if _, exists := clients[c]; !exists {
		return
	}
	delete(clients, c)
	if len(clients) == 0 {
		delete(h.clientsByUser, c.userID)
	}
	h.totalConnections--
	close(c.send)
}

func (h *WSHub) PublishToUser(userID string, payload []byte) {
	h.mu.RLock()
	clients := h.clientsByUser[userID]
	copied := make([]*wsClient, 0, len(clients))
	for c := range clients {
		copied = append(copied, c)
	}
	h.mu.RUnlock()

	if len(copied) == 0 {
		log.Printf("[ws] PublishToUser: nenhum cliente WebSocket ligado para userId=%s (mensagem não entregue; abra GET /v1/ws com o JWT deste utilizador)", userID)
		return
	}

	for _, c := range copied {
		select {
		case c.send <- wsMessage{messageType: websocket.TextMessage, payload: payload}:
			h.mu.Lock()
			h.messagesSent++
			h.mu.Unlock()
		default:
			log.Printf("[ws] dropping slow connection connectionId=%s userId=%s", c.connectionID, c.userID)
			h.mu.Lock()
			h.droppedByBackpress++
			h.mu.Unlock()
			_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "connection too slow"), time.Now().Add(2*time.Second))
			_ = c.conn.Close()
		}
	}
}

func (h *WSHub) ActiveConnections(userID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clientsByUser[userID])
}

func (h *WSHub) IncrementWriteError() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writeErrors++
}

type WSHubStats struct {
	ActiveUsers        int   `json:"activeUsers"`
	TotalConnections   int64 `json:"totalConnections"`
	DroppedByBackpress int64 `json:"droppedByBackpressure"`
	MessagesSent       int64 `json:"messagesSent"`
	WriteErrors        int64 `json:"writeErrors"`
}

func (h *WSHub) Stats() WSHubStats {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return WSHubStats{
		ActiveUsers:        len(h.clientsByUser),
		TotalConnections:   h.totalConnections,
		DroppedByBackpress: h.droppedByBackpress,
		MessagesSent:       h.messagesSent,
		WriteErrors:        h.writeErrors,
	}
}
