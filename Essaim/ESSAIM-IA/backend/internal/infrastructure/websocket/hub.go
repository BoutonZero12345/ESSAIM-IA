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

// Hub manages connected WebSocket clients and broadcasts events.
type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]bool
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]bool),
	}
}

// HandleWebSocket upgrades HTTP connections to WebSocket and registers them.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS HUB] Upgrade error: %v", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	log.Printf("[WS HUB] New client connected (%d total)", h.ClientCount())

	// Keep connection alive and handle disconnect
	go h.readPump(conn)
}

// readPump listens for client messages (mainly to detect disconnect).
func (h *Hub) readPump(conn *websocket.Conn) {
	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
		log.Printf("[WS HUB] Client disconnected (%d remaining)", h.ClientCount())
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
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
	defer h.mu.RUnlock()

	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[WS HUB] Write error: %v", err)
			conn.Close()
			delete(h.clients, conn)
		}
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
