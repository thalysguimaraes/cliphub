package hub

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

// Config holds hub configuration.
type Config struct {
	MaxHistory int
	TTL        time.Duration
	DBPath     string // Empty = no persistence.
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
	store      clipStore // nil if no persistence.

	// publishMu serializes post-commit side effects in sequence order so
	// persistence and subscriber delivery stay monotonic after h.mu is released.
	publishMu    sync.Mutex
	publishCond  *sync.Cond
	publishedSeq uint64

	subsMu sync.RWMutex
	subs   map[*Subscriber]struct{}
}

type clipStore interface {
	Close() error
	LoadState(maxHistory int) (uint64, []protocol.ClipItem, error)
	HistoryPage(limit int, beforeSeq uint64) ([]protocol.ClipItem, error)
	LoadItem(seq uint64) (*protocol.ClipItem, error)
	SaveItem(item protocol.ClipItem) (protocol.ClipItem, error)
	DeleteExpired(before time.Time) (int, error)
	DeleteAll() error
}

// New creates a Hub, optionally backed by SQLite, and starts the TTL reaper.
func New(cfg Config) (*Hub, error) {
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

	if cfg.DBPath != "" {
		st, err := OpenStore(cfg.DBPath)
		if err != nil {
			return nil, fmt.Errorf("open clip store: %w", err)
		}
		h.store = st
		seq, items, err := st.LoadState(cfg.MaxHistory)
		if err != nil {
			st.Close()
			return nil, fmt.Errorf("load clip state: %w", err)
		}
		h.seq = seq
		h.history = items
		if len(items) > 0 {
			h.current = &items[0]
		}
		slog.Info("loaded state from db", "component", "hub_store", "sequence", seq, "history_items", len(items))
	}

	h.publishCond = sync.NewCond(&h.publishMu)
	h.publishedSeq = h.seq

	go h.reapLoop()
	return h, nil
}

// Close shuts down the hub's persistent store.
func (h *Hub) Close() error {
	if h.store != nil {
		return h.store.Close()
	}
	return nil
}

// PutInput describes a new clipboard item to store.
type PutInput struct {
	MimeType string
	Content  string // For text/* types.
	Data     []byte // For binary types.
	Source   string
}

// Put stores a new clipboard item. Returns the item and true if it was new,
// or the existing current item and false if it was a duplicate.
func (h *Hub) Put(in PutInput) (protocol.ClipItem, bool) {
	var hash string
	if strings.HasPrefix(in.MimeType, "text/") {
		hash = protocol.HashContent(in.Content)
	} else {
		hash = protocol.HashBytes(in.Data)
	}

	h.mu.Lock()
	if h.current != nil && h.current.Hash == hash && h.current.MimeType == in.MimeType {
		item := cloneClipItem(*h.current)
		h.mu.Unlock()
		return item, false
	}

	h.seq++
	now := time.Now()
	item := protocol.ClipItem{
		Seq:       h.seq,
		MimeType:  in.MimeType,
		Content:   in.Content,
		Data:      cloneBytes(in.Data),
		Hash:      hash,
		Source:    in.Source,
		CreatedAt: now,
		ExpiresAt: now.Add(h.ttl),
	}

	h.current = &item
	h.history = append([]protocol.ClipItem{item}, h.history...)
	if len(h.history) > h.maxHistory {
		h.history = h.history[:h.maxHistory]
	}
	h.mu.Unlock()
	h.publish(item)
	return cloneClipItem(item), true
}

// Get returns the current clipboard item, or nil.
func (h *Hub) Get() *protocol.ClipItem {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return cloneClipItemPtr(h.current)
}

// History returns up to limit items from history.
func (h *Hub) History(limit int) []protocol.ClipItem {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if limit <= 0 || limit > len(h.history) {
		limit = len(h.history)
	}
	return cloneClipItems(h.history[:limit])
}

// HistoryPage returns a cursor-addressable page of history items, newest-first.
// When a persistent store is available, paging can extend beyond the in-memory history window.
func (h *Hub) HistoryPage(limit int, beforeSeq uint64) ([]protocol.ClipItem, string, bool, error) {
	if limit <= 0 {
		limit = protocol.DefaultHistoryLimit
	}

	pageLimit := limit + 1
	var items []protocol.ClipItem
	var err error

	if h.store != nil {
		items, err = h.store.HistoryPage(pageLimit, beforeSeq)
		if err != nil {
			return nil, "", false, err
		}
	} else {
		items = h.historyPageFromMemory(pageLimit, beforeSeq)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if hasMore && len(items) > 0 {
		nextCursor = strconv.FormatUint(items[len(items)-1].Seq, 10)
	}
	return cloneClipItems(items), nextCursor, hasMore, nil
}

// Clear removes the current clip and persisted history while preserving the
// sequence counter for future writes.
func (h *Hub) Clear() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.store != nil {
		if err := h.store.DeleteAll(); err != nil {
			return err
		}
	}

	h.current = nil
	h.history = nil
	return nil
}

// Since returns all items in history with seq > afterSeq, in chronological order.
func (h *Hub) Since(afterSeq uint64) []protocol.ClipItem {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var out []protocol.ClipItem
	for i := len(h.history) - 1; i >= 0; i-- {
		if h.history[i].Seq > afterSeq {
			out = append(out, cloneClipItem(h.history[i]))
		}
	}
	return out
}

// GetBySeq returns a historical clip by sequence number, or nil when not found.
func (h *Hub) GetBySeq(seq uint64) (*protocol.ClipItem, error) {
	h.mu.RLock()
	if h.current != nil && h.current.Seq == seq {
		item := cloneClipItem(*h.current)
		h.mu.RUnlock()
		return &item, nil
	}
	for _, item := range h.history {
		if item.Seq == seq {
			cloned := cloneClipItem(item)
			h.mu.RUnlock()
			return &cloned, nil
		}
	}
	h.mu.RUnlock()

	if h.store == nil {
		return nil, nil
	}
	return h.store.LoadItem(seq)
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

func (h *Hub) publish(item protocol.ClipItem) {
	if h.publishCond == nil {
		h.publishCond = sync.NewCond(&h.publishMu)
		h.publishedSeq = h.seq - 1
	}

	h.publishMu.Lock()
	for item.Seq != h.publishedSeq+1 {
		h.publishCond.Wait()
	}

	for _, sub := range h.snapshotSubscribers() {
		select {
		case sub.C <- cloneClipItem(item):
		default:
		}
	}

	if h.store != nil {
		if _, err := h.store.SaveItem(item); err != nil {
			slog.Error("failed to persist clip", "component", "hub_store", "sequence", item.Seq, "error", err)
		}
	}

	h.publishedSeq = item.Seq
	h.publishCond.Broadcast()
	h.publishMu.Unlock()
}

func (h *Hub) snapshotSubscribers() []*Subscriber {
	h.subsMu.RLock()
	defer h.subsMu.RUnlock()

	subs := make([]*Subscriber, 0, len(h.subs))
	for sub := range h.subs {
		subs = append(subs, sub)
	}
	return subs
}

func cloneClipItemPtr(item *protocol.ClipItem) *protocol.ClipItem {
	if item == nil {
		return nil
	}
	cloned := cloneClipItem(*item)
	return &cloned
}

func cloneClipItems(items []protocol.ClipItem) []protocol.ClipItem {
	cloned := make([]protocol.ClipItem, len(items))
	for i := range items {
		cloned[i] = cloneClipItem(items[i])
	}
	return cloned
}

func cloneClipItem(item protocol.ClipItem) protocol.ClipItem {
	item.Data = cloneBytes(item.Data)
	return item
}

func (h *Hub) historyPageFromMemory(limit int, beforeSeq uint64) []protocol.ClipItem {
	h.mu.RLock()
	defer h.mu.RUnlock()

	start := 0
	if beforeSeq > 0 {
		start = len(h.history)
		for i, item := range h.history {
			if item.Seq < beforeSeq {
				start = i
				break
			}
		}
	}
	if start >= len(h.history) {
		return nil
	}

	end := start + limit
	if end > len(h.history) {
		end = len(h.history)
	}
	return cloneClipItems(h.history[start:end])
}

func cloneBytes(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	return append([]byte(nil), data...)
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
		slog.Info("current clip expired", "component", "hub_ttl")
	}

	kept := h.history[:0]
	for _, item := range h.history {
		if !now.After(item.ExpiresAt) {
			kept = append(kept, item)
		}
	}
	if reaped := len(h.history) - len(kept); reaped > 0 {
		slog.Info("reaped expired clips", "component", "hub_ttl", "expired_items", reaped)
	}
	h.history = kept

	if h.store != nil {
		go func() {
			if n, err := h.store.DeleteExpired(now); err != nil {
				slog.Error("failed to delete expired from db", "component", "hub_store", "error", err)
			} else if n > 0 {
				slog.Info("reaped expired clips from db", "component", "hub_store", "expired_items", n)
			}
		}()
	}
}
