package hub

import (
	"context"
	"testing"
	"time"
)

func newTestHub() *Hub {
	return &Hub{
		maxHistory: 5,
		ttl:        time.Hour,
		subs:       make(map[*Subscriber]struct{}),
		startedAt:  time.Now(),
	}
}

func textInput(content, source string) PutInput {
	return PutInput{MimeType: "text/plain", Content: content, Source: source}
}

func TestPutAndGet(t *testing.T) {
	h := newTestHub()

	item, isNew := h.Put(textInput("hello", "node1"))
	if !isNew {
		t.Fatal("first put should be new")
	}
	if item.Seq != 1 {
		t.Fatalf("expected seq 1, got %d", item.Seq)
	}
	if item.Content != "hello" || item.MimeType != "text/plain" {
		t.Fatal("content mismatch")
	}

	got := h.Get()
	if got == nil || got.Seq != 1 {
		t.Fatal("Get should return current item")
	}
}

func TestPutBinary(t *testing.T) {
	h := newTestHub()

	item, isNew := h.Put(PutInput{
		MimeType: "image/png",
		Data:     []byte{0x89, 0x50, 0x4e, 0x47},
		Source:   "node1",
	})
	if !isNew {
		t.Fatal("first put should be new")
	}
	if item.MimeType != "image/png" || len(item.Data) != 4 {
		t.Fatal("binary data mismatch")
	}
}

func TestDedup(t *testing.T) {
	h := newTestHub()

	h.Put(textInput("hello", "node1"))
	_, isNew := h.Put(textInput("hello", "node2"))
	if isNew {
		t.Fatal("duplicate content should not be new")
	}
	if h.Seq() != 1 {
		t.Fatal("seq should not increment on dedup")
	}
}

func TestDedupBinary(t *testing.T) {
	h := newTestHub()

	data := []byte{1, 2, 3}
	h.Put(PutInput{MimeType: "image/png", Data: data, Source: "a"})
	_, isNew := h.Put(PutInput{MimeType: "image/png", Data: data, Source: "b"})
	if isNew {
		t.Fatal("duplicate binary should not be new")
	}
}

func TestMonotonicSeq(t *testing.T) {
	h := newTestHub()

	for i := 0; i < 10; i++ {
		item, _ := h.Put(textInput(string(rune('a'+i)), "node1"))
		if item.Seq != uint64(i+1) {
			t.Fatalf("expected seq %d, got %d", i+1, item.Seq)
		}
	}
}

func TestHistoryRingBuffer(t *testing.T) {
	h := newTestHub()

	for i := 0; i < 10; i++ {
		h.Put(textInput(string(rune('a'+i)), "node1"))
	}

	hist := h.History(0)
	if len(hist) != 5 {
		t.Fatalf("expected 5 history items (maxHistory), got %d", len(hist))
	}
	if hist[0].Seq != 10 {
		t.Fatalf("expected most recent seq 10, got %d", hist[0].Seq)
	}
}

func TestHistoryLimit(t *testing.T) {
	h := newTestHub()

	for i := 0; i < 5; i++ {
		h.Put(textInput(string(rune('a'+i)), "node1"))
	}

	hist := h.History(3)
	if len(hist) != 3 {
		t.Fatalf("expected 3 items, got %d", len(hist))
	}
}

func TestSubscriberReceivesUpdates(t *testing.T) {
	h := newTestHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := h.Subscribe(ctx)

	h.Put(textInput("hello", "node1"))

	select {
	case item := <-sub.C:
		if item.Content != "hello" {
			t.Fatal("wrong content")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for subscriber update")
	}
}

func TestSubscriberCountAfterCancel(t *testing.T) {
	h := newTestHub()
	ctx, cancel := context.WithCancel(context.Background())

	h.Subscribe(ctx)
	if h.SubscriberCount() != 1 {
		t.Fatal("expected 1 subscriber")
	}

	cancel()
	time.Sleep(50 * time.Millisecond)

	if h.SubscriberCount() != 0 {
		t.Fatal("expected 0 subscribers after cancel")
	}
}

func TestReapExpired(t *testing.T) {
	h := &Hub{
		maxHistory: 50,
		ttl:        time.Millisecond,
		subs:       make(map[*Subscriber]struct{}),
		startedAt:  time.Now(),
	}

	h.Put(textInput("ephemeral", "node1"))
	time.Sleep(10 * time.Millisecond)
	h.reapExpired()

	if h.Get() != nil {
		t.Fatal("expected current to be nil after expiry")
	}
	if len(h.History(0)) != 0 {
		t.Fatal("expected empty history after expiry")
	}
}

func TestGetEmpty(t *testing.T) {
	h := newTestHub()
	if h.Get() != nil {
		t.Fatal("expected nil on empty hub")
	}
}
