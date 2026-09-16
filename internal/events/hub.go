package events

import (
	"encoding/json"
	"sync"
	"time"
)

type Event struct {
	Type     string    `json:"type"`
	DeviceID string    `json:"device_id,omitempty"`
	Data     any       `json:"data,omitempty"`
	Time     time.Time `json:"time"`
}

type Hub struct {
	mu      sync.RWMutex
	nextID  int
	clients map[int]chan Event
}

func New() *Hub { return &Hub{clients: map[int]chan Event{}} }

func (h *Hub) Subscribe() (int, <-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := h.nextID
	h.nextID++
	ch := make(chan Event, 32)
	h.clients[id] = ch
	return id, ch, func() {
		h.mu.Lock()
		if c, ok := h.clients[id]; ok {
			delete(h.clients, id)
			close(c)
		}
		h.mu.Unlock()
	}
}

func (h *Hub) Publish(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.clients {
		select {
		case ch <- e:
		default:
		}
	}
}

func EncodeSSE(e Event) []byte {
	b, _ := json.Marshal(e)
	return append(append([]byte("data: "), b...), []byte("\n\n")...)
}
