package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// EventType defines the WebSocket event types per Architecture §10.3.
type EventType string

const (
	EventGraphUpdate EventType = "GRAPH_UPDATE"
	EventAgentState  EventType = "AGENT_STATE"
	EventLogStream   EventType = "LOG_STREAM"
	EventSystemAlert EventType = "SYSTEM_ALERT"
)

// Event is the JSON payload sent to frontend clients.
type Event struct {
	Type    EventType   `json:"type"`
	Payload interface{} `json:"payload"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins for dev
}

// safeConn wraps a websocket connection with a write mutex to prevent concurrent writes.
type safeConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// writeMessage safely writes a message, serializing concurrent writes.
func (sc *safeConn) writeMessage(messageType int, data []byte) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.conn.WriteMessage(messageType, data)
}

// Hub manages connected WebSocket clients and broadcasts events.
type Hub struct {
	mu      sync.RWMutex
	clients map[*safeConn]bool
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*safeConn]bool),
	}
}

// HandleWebSocket upgrades HTTP connections to WebSocket and registers them.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS HUB] Upgrade error: %v", err)
		return
	}

	sc := &safeConn{conn: conn}

	h.mu.Lock()
	h.clients[sc] = true
	h.mu.Unlock()

	log.Printf("[WS HUB] New client connected (%d total)", h.ClientCount())

	// Keep connection alive and handle disconnect
	go h.readPump(sc)
}

// readPump listens for client messages (mainly to detect disconnect).
func (h *Hub) readPump(sc *safeConn) {
	defer func() {
		h.mu.Lock()
		delete(h.clients, sc)
		h.mu.Unlock()
		sc.conn.Close()
		log.Printf("[WS HUB] Client disconnected (%d remaining)", h.ClientCount())
	}()

	for {
		if _, _, err := sc.conn.ReadMessage(); err != nil {
			break
		}
	}
}

// Broadcast sends an event to all connected clients.
func (h *Hub) Broadcast(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[WS HUB] Failed to marshal event: %v", err)
		return
	}

	h.mu.RLock()
	clients := make([]*safeConn, 0, len(h.clients))
	for sc := range h.clients {
		clients = append(clients, sc)
	}
	h.mu.RUnlock()

	for _, sc := range clients {
		if err := sc.writeMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[WS HUB] Write error: %v", err)
			h.mu.Lock()
			delete(h.clients, sc)
			h.mu.Unlock()
			sc.conn.Close()
		}
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
