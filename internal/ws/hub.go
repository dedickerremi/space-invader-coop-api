package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// connEntry wraps a connection with a write mutex to prevent concurrent writes.
type connEntry struct {
	conn *websocket.Conn
	wmu  sync.Mutex
}

// Hub holds connected clients per match and provides broadcast.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[string]*connEntry // matchID -> playerID -> connEntry
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[string]*connEntry),
	}
}

// Register adds a client for the given match and player.
func (h *Hub) Register(matchID, playerID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[matchID] == nil {
		h.clients[matchID] = make(map[string]*connEntry)
	}
	h.clients[matchID][playerID] = &connEntry{conn: conn}
}

// Unregister removes a client ONLY if the stored connection matches the given one.
// Returns the remaining count for the match, or -1 if the conn didn't match (stale).
func (h *Hub) Unregister(matchID, playerID string, conn *websocket.Conn) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	m, ok := h.clients[matchID]
	if !ok {
		return 0
	}
	entry, exists := m[playerID]
	if !exists {
		return len(m)
	}
	// Only remove if the stored connection is the same as the one disconnecting
	if entry.conn != conn {
		return -1 // stale goroutine
	}
	delete(m, playerID)
	n := len(m)
	if n == 0 {
		delete(h.clients, matchID)
	}
	return n
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
	// Copy entries so we don't hold lock while writing
	entries := make([]*connEntry, 0, len(m))
	for _, e := range m {
		entries = append(entries, e)
	}
	h.mu.RUnlock()

	for _, e := range entries {
		if e != nil && e.conn != nil {
			e.wmu.Lock()
			if err := e.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				fmt.Printf("[WS] Write error in broadcast to %s: %v\n", matchID, err)
			}
			e.wmu.Unlock()
		}
	}
}

// Send sends a JSON message to a single client (by raw conn pointer).
// For sends that happen inline in HandleConnection, use SendEntry instead when possible.
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

// SendSafe sends a JSON message using the per-connection write mutex.
func (h *Hub) SendSafe(matchID, playerID string, msg interface{}) {
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
	entry, ok := m[playerID]
	h.mu.RUnlock()
	if !ok || entry == nil {
		return
	}
	entry.wmu.Lock()
	_ = entry.conn.WriteMessage(websocket.TextMessage, data)
	entry.wmu.Unlock()
}

// CloseMatch closes all connections in the match (e.g. on exit or disconnect).
func (h *Hub) CloseMatch(matchID string) {
	h.mu.Lock()
	m, ok := h.clients[matchID]
	if !ok {
		h.mu.Unlock()
		return
	}
	entries := make([]*connEntry, 0, len(m))
	for _, e := range m {
		entries = append(entries, e)
	}
	delete(h.clients, matchID)
	h.mu.Unlock()
	for _, e := range entries {
		if e != nil && e.conn != nil {
			_ = e.conn.Close()
		}
	}
}

// GetConnectedCount returns the number of connected clients in a match.
func (h *Hub) GetConnectedCount(matchID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[matchID])
}

// HasPlayer returns true if the given player is registered in the match.
func (h *Hub) HasPlayer(matchID, playerID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	m, ok := h.clients[matchID]
	if !ok {
		return false
	}
	_, exists := m[playerID]
	return exists
}
