package hub

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/thalys/cliphub/internal/protocol"
)

// Config holds hub configuration.
type Config struct {
	MaxHistory int
	TTL        time.Duration
}

// Subscriber receives clipboard updates via a channel.
type Subscriber struct {
	C      chan protocol.ClipItem
	cancel context.CancelFunc
}

// Hub is the central clipboard broker.
type Hub struct {
	mu         sync.RWMutex
	current    *protocol.ClipItem
	history    []protocol.ClipItem
	maxHistory int
	seq        uint64
	ttl        time.Duration
	startedAt  time.Time

	subsMu sync.RWMutex
	subs   map[*Subscriber]struct{}
}

// New creates a Hub and starts the TTL reaper.
func New(cfg Config) *Hub {
	if cfg.MaxHistory <= 0 {
		cfg.MaxHistory = 50
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	h := &Hub{
		maxHistory: cfg.MaxHistory,
		ttl:        cfg.TTL,
		subs:       make(map[*Subscriber]struct{}),
		startedAt:  time.Now(),
	}
	go h.reapLoop()
	return h
}

// Put stores a new clipboard item. Returns the item and true if it was new,
// or the existing current item and false if it was a duplicate.
func (h *Hub) Put(content, source string) (protocol.ClipItem, bool) {
	hash := protocol.HashContent(content)

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.current != nil && h.current.Hash == hash {
		return *h.current, false
	}

	h.seq++
	now := time.Now()
	item := protocol.ClipItem{
		Seq:       h.seq,
		Content:   content,
		Hash:      hash,
		Source:    source,
		CreatedAt: now,
		ExpiresAt: now.Add(h.ttl),
	}

	h.current = &item
	h.history = append([]protocol.ClipItem{item}, h.history...)
	if len(h.history) > h.maxHistory {
		h.history = h.history[:h.maxHistory]
	}

	// Fan-out to subscribers (non-blocking).
	h.subsMu.RLock()
	for sub := range h.subs {
		select {
		case sub.C <- item:
		default:
			// Slow subscriber, drop update.
		}
	}
	h.subsMu.RUnlock()

	return item, true
}

// Get returns the current clipboard item, or nil.
func (h *Hub) Get() *protocol.ClipItem {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current
}

// History returns up to limit items from history.
func (h *Hub) History(limit int) []protocol.ClipItem {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if limit <= 0 || limit > len(h.history) {
		limit = len(h.history)
	}
	out := make([]protocol.ClipItem, limit)
	copy(out, h.history[:limit])
	return out
}

// Subscribe creates a new subscriber. Cancel the context to unsubscribe.
func (h *Hub) Subscribe(ctx context.Context) *Subscriber {
	ctx, cancel := context.WithCancel(ctx)
	sub := &Subscriber{
		C:      make(chan protocol.ClipItem, 16),
		cancel: cancel,
	}

	h.subsMu.Lock()
	h.subs[sub] = struct{}{}
	h.subsMu.Unlock()

	go func() {
		<-ctx.Done()
		h.unsubscribe(sub)
	}()

	return sub
}

func (h *Hub) unsubscribe(sub *Subscriber) {
	h.subsMu.Lock()
	delete(h.subs, sub)
	h.subsMu.Unlock()
}

// SubscriberCount returns the number of active subscribers.
func (h *Hub) SubscriberCount() int {
	h.subsMu.RLock()
	defer h.subsMu.RUnlock()
	return len(h.subs)
}

// Seq returns the current sequence number.
func (h *Hub) Seq() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.seq
}

// StartedAt returns when the hub was created.
func (h *Hub) StartedAt() time.Time {
	return h.startedAt
}

func (h *Hub) reapLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		h.reapExpired()
	}
}

func (h *Hub) reapExpired() {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.current != nil && now.After(h.current.ExpiresAt) {
		h.current = nil
		slog.Info("current clip expired")
	}

	kept := h.history[:0]
	for _, item := range h.history {
		if !now.After(item.ExpiresAt) {
			kept = append(kept, item)
		}
	}
	if reaped := len(h.history) - len(kept); reaped > 0 {
		slog.Info("reaped expired clips", "count", reaped)
	}
	h.history = kept
}
