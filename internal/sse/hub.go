// Package sse implements the server-push transport that is the only
// realtime channel in k8s-driller (SPECS.md §2, §6.2 — SSE-only, no
// WebSockets). Each topic gets its own broadcast fan-out; a new subscriber
// always receives a full snapshot before any patch, so it never has to
// reconcile partial state.
package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/TaliaMarine/k8s-driller/internal/appmetrics"
)

// EventType distinguishes a full state snapshot from an incremental patch,
// per SPECS.md §6.2 (clients apply the same reducer either way).
type EventType string

const (
	EventSnapshot EventType = "snapshot"
	EventPatch    EventType = "patch"
)

// Event is one message published to a topic.
type Event struct {
	Type EventType
	Data any
}

type subscriber struct {
	events chan Event
}

// Topic is a single broadcast channel (e.g. "cluster", "node:worker-1",
// "alerts" — SPECS.md §6.2).
type Topic struct {
	mu          sync.Mutex
	subscribers map[*subscriber]struct{}
	// snapshot is re-sent to every new subscriber before any patch.
	lastSnapshot any
}

func newTopic() *Topic {
	return &Topic{subscribers: make(map[*subscriber]struct{})}
}

// Publish sends ev to every current subscriber of this topic. A snapshot
// event also becomes the topic's cached snapshot for future subscribers.
func (t *Topic) Publish(ev Event) {
	t.mu.Lock()
	if ev.Type == EventSnapshot {
		t.lastSnapshot = ev.Data
	}
	subs := make([]*subscriber, 0, len(t.subscribers))
	for s := range t.subscribers {
		subs = append(subs, s)
	}
	t.mu.Unlock()

	for _, s := range subs {
		select {
		case s.events <- ev:
		default:
			// Slow consumer: drop rather than block the publisher. The
			// client's next reconnect gets a fresh snapshot (SPECS.md §7.3).
		}
	}
}

func (t *Topic) subscribe() *subscriber {
	s := &subscriber{events: make(chan Event, 32)}
	t.mu.Lock()
	t.subscribers[s] = struct{}{}
	snapshot := t.lastSnapshot
	t.mu.Unlock()
	if snapshot != nil {
		s.events <- Event{Type: EventSnapshot, Data: snapshot}
	}
	return s
}

func (t *Topic) unsubscribe(s *subscriber) {
	t.mu.Lock()
	delete(t.subscribers, s)
	t.mu.Unlock()
	close(s.events)
}

// Hub owns every topic and serves the SSE HTTP handler.
type Hub struct {
	mu        sync.Mutex
	topics    map[string]*Topic
	keepalive time.Duration
}

// New builds a Hub. keepalive is the interval between SSE comment pings that
// keep proxies/ingress from timing out idle connections (SPECS.md §4.1 SSE
// hub).
func New(keepalive time.Duration) *Hub {
	return &Hub{topics: make(map[string]*Topic), keepalive: keepalive}
}

func (h *Hub) topic(name string) *Topic {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.topics[name]
	if !ok {
		t = newTopic()
		h.topics[name] = t
	}
	return t
}

// Publish sends ev to every subscriber of the named topic.
func (h *Hub) Publish(topicName string, ev Event) {
	h.topic(topicName).Publish(ev)
}

// PublishSnapshot is a convenience wrapper for the common case of pushing a
// full snapshot.
func (h *Hub) PublishSnapshot(topicName string, data any) {
	h.Publish(topicName, Event{Type: EventSnapshot, Data: data})
}

// PublishPatch is a convenience wrapper for the common case of pushing an
// incremental patch.
func (h *Hub) PublishPatch(topicName string, data any) {
	h.Publish(topicName, Event{Type: EventPatch, Data: data})
}

// ServeHTTP streams the named topic to the client as SSE, per SPECS.md §6.2.
// topicName is fixed per call site (e.g. wired to "cluster" or
// "node:"+name), not taken from the request, so a handler can enforce its
// own auth/role check before ever reaching this method.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request, topicName string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sub := h.topic(topicName).subscribe()
	appmetrics.SSEClientsConnected.WithLabelValues(topicName).Inc()
	defer func() {
		h.topic(topicName).unsubscribe(sub)
		appmetrics.SSEClientsConnected.WithLabelValues(topicName).Dec()
	}()

	ticker := time.NewTicker(h.keepalive)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-sub.events:
			if !open {
				return
			}
			if err := writeEvent(w, ev); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, ev Event) error {
	payload, err := json.Marshal(ev.Data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload); err != nil {
		return err
	}
	return nil
}
