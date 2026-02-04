package ws

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Hub holds connected clients per match and provides broadcast.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[string]*websocket.Conn // matchID -> playerID -> conn
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[string]*websocket.Conn),
	}
}

// Register adds a client for the given match and player.
func (h *Hub) Register(matchID, playerID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[matchID] == nil {
		h.clients[matchID] = make(map[string]*websocket.Conn)
	}
	h.clients[matchID][playerID] = conn
}

// Unregister removes a client and returns the remaining count for the match.
func (h *Hub) Unregister(matchID, playerID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m, ok := h.clients[matchID]; ok {
		delete(m, playerID)
		n := len(m)
		if n == 0 {
			delete(h.clients, matchID)
		}
		return n
	}
	return 0
}

// BroadcastToMatch sends a JSON message to all clients in the match.
func (h *Hub) BroadcastToMatch(matchID string, msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WS] Marshal error: %v", err)
		return
	}
	h.mu.RLock()
	m, ok := h.clients[matchID]
	if !ok {
		h.mu.RUnlock()
		return
	}
	// Copy conns so we don't hold lock while writing
	conns := make([]*websocket.Conn, 0, len(m))
	for _, c := range m {
		conns = append(conns, c)
	}
	h.mu.RUnlock()
	for _, conn := range conns {
		if conn != nil {
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}
	}
}

// Send sends a JSON message to a single client.
func (h *Hub) Send(conn *websocket.Conn, msg interface{}) {
	if conn == nil {
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WS] Marshal error: %v", err)
		return
	}
	_ = conn.WriteMessage(websocket.TextMessage, data)
}

// CloseMatch closes all connections in the match (e.g. on exit or disconnect).
func (h *Hub) CloseMatch(matchID string) {
	h.mu.Lock()
	m, ok := h.clients[matchID]
	if !ok {
		h.mu.Unlock()
		return
	}
	conns := make([]*websocket.Conn, 0, len(m))
	for _, c := range m {
		conns = append(conns, c)
	}
	delete(h.clients, matchID)
	h.mu.Unlock()
	for _, conn := range conns {
		if conn != nil {
			_ = conn.Close()
		}
	}
}

// GetConnectedCount returns the number of connected clients in a match.
func (h *Hub) GetConnectedCount(matchID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[matchID])
}
