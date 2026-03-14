package internal

import (
	"sync"

	"github.com/gorilla/websocket"
)

// Hub manages WebSocket connections grouped by job ID.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[*websocket.Conn]struct{}),
	}
}

// Register adds a connection to the job's subscriber set.
func (h *Hub) Register(jobID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[jobID] == nil {
		h.clients[jobID] = make(map[*websocket.Conn]struct{})
	}
	h.clients[jobID][conn] = struct{}{}
}

// Unregister removes a connection and closes it.
func (h *Hub) Unregister(jobID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.clients[jobID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.clients, jobID)
		}
	}
	conn.Close()
}

// Broadcast sends a JSON-serialisable message to all clients of a job.
func (h *Hub) Broadcast(jobID string, msg any) {
	h.mu.RLock()
	conns := h.clients[jobID]
	h.mu.RUnlock()

	for conn := range conns {
		if err := conn.WriteJSON(msg); err != nil {
			h.Unregister(jobID, conn)
		}
	}
}
