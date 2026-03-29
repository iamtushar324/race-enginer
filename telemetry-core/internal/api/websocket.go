package api

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/rs/zerolog/log"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
)

// wsMessage is the typed JSON envelope sent to WebSocket clients.
type wsMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Client represents a single WebSocket connection.
type Client struct {
	conn *websocket.Conn
	send chan []byte
}

// Hub manages all active WebSocket clients and broadcasts data to them.
type Hub struct {
	clients    map[*Client]bool
	mu         sync.RWMutex
	pushRateHz int
	cacheLoad  func() *models.RaceState
	healthFn   func() interface{}
}

// NewHub creates a WebSocket hub.
// pushRateHz controls how often telemetry is pushed (e.g. 10 = every 100ms).
// cacheLoad reads the atomic RaceState cache.
// healthFn builds the health payload.
func NewHub(pushRateHz int, cacheLoad func() *models.RaceState, healthFn func() interface{}) *Hub {
	if pushRateHz < 1 {
		pushRateHz = 1
	}
	if pushRateHz > 60 {
		pushRateHz = 60
	}
	return &Hub{
		clients:    make(map[*Client]bool),
		pushRateHz: pushRateHz,
		cacheLoad:  cacheLoad,
		healthFn:   healthFn,
	}
}

// register adds a client to the hub.
func (h *Hub) register(c *Client) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	log.Info().Int("clients", len(h.clients)).Msg("WebSocket client connected")
}

// unregister removes a client from the hub and closes its send channel.
func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
	log.Info().Int("clients", len(h.clients)).Msg("WebSocket client disconnected")
}

// broadcast sends a pre-marshaled message to all connected clients.
// Non-blocking: if a client's send buffer is full, it is disconnected.
func (h *Hub) broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		select {
		case c.send <- data:
		default:
			// Slow consumer — disconnect.
			go h.unregister(c)
		}
	}
}

// BroadcastAudio sends base64-encoded audio to all WebSocket clients.
func (h *Hub) BroadcastAudio(audioBase64 string, format string) {
	msg := struct {
		Type   string `json:"type"`
		Data   string `json:"data"`
		Format string `json:"format"`
	}{
		Type:   "audio",
		Data:   audioBase64,
		Format: format,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal WS audio")
		return
	}
	h.broadcast(data)
}

// BroadcastPTT sends a push-to-talk state change to all connected WebSocket clients.
func (h *Hub) BroadcastPTT(active bool) {
	msg := wsMessage{Type: "ptt", Data: map[string]bool{"active": active}}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal WS PTT")
		return
	}
	h.broadcast(data)
}

// BroadcastInsight sends an insight to all connected WebSocket clients.
func (h *Hub) BroadcastInsight(insight models.DrivingInsight) {
	msg := wsMessage{Type: "insight", Data: insight}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal WS insight")
		return
	}
	h.broadcast(data)
}

// Run starts the hub's broadcast loops. It blocks until ctx is cancelled.
func (h *Hub) Run(ctx context.Context) {
	telemetryInterval := time.Duration(float64(time.Second) / float64(h.pushRateHz))
	telemetryTicker := time.NewTicker(telemetryInterval)
	defer telemetryTicker.Stop()

	healthTicker := time.NewTicker(5 * time.Second)
	defer healthTicker.Stop()

	log.Info().Int("push_rate_hz", h.pushRateHz).Dur("interval", telemetryInterval).Msg("WebSocket hub started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("WebSocket hub stopping")
			return

		case <-telemetryTicker.C:
			state := h.cacheLoad()
			if state == nil {
				continue
			}
			msg := wsMessage{Type: "telemetry", Data: state}
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			h.broadcast(data)

		case <-healthTicker.C:
			health := h.healthFn()
			msg := wsMessage{Type: "health", Data: health}
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			h.broadcast(data)
		}
	}
}

// wsHandler returns a Fiber WebSocket handler that manages a single client connection.
func wsHandler(hub *Hub) func(*websocket.Conn) {
	return func(conn *websocket.Conn) {
		client := &Client{
			conn: conn,
			send: make(chan []byte, 256),
		}
		hub.register(client)
		defer hub.unregister(client)

		// Writer goroutine: drains send channel and writes to WebSocket.
		go func() {
			defer conn.Close()
			for msg := range client.send {
				if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			}
		}()

		// Reader loop: keeps the connection alive (reads pong frames).
		// We don't expect client messages, but must drain reads to detect close.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}
}
