package hub

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

func newTestHub() *Hub {
	h := &Hub{
		maxHistory: 5,
		ttl:        time.Hour,
		subs:       make(map[*Subscriber]struct{}),
		startedAt:  time.Now(),
	}
	h.publishCond = sync.NewCond(&h.publishMu)
	return h
}

func textInput(content, source string) PutInput {
	return PutInput{MimeType: "text/plain", Content: content, Source: source}
}

type blockingStore struct {
	saveStarted chan protocol.ClipItem
	releaseSave chan struct{}
}

func newBlockingStore() *blockingStore {
	return &blockingStore{
		saveStarted: make(chan protocol.ClipItem, 8),
		releaseSave: make(chan struct{}, 8),
	}
}

func (s *blockingStore) Close() error { return nil }

func (s *blockingStore) LoadState(int) (uint64, []protocol.ClipItem, error) {
	return 0, nil, nil
}

func (s *blockingStore) SaveItem(item protocol.ClipItem) error {
	s.saveStarted <- item
	<-s.releaseSave
	return nil
}

func (s *blockingStore) DeleteExpired(time.Time) (int, error) {
	return 0, nil
}

func (s *blockingStore) allowOneSave() {
	s.releaseSave <- struct{}{}
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

func TestGetReturnsDeepCopy(t *testing.T) {
	h := newTestHub()

	original := []byte{1, 2, 3, 4}
	h.Put(PutInput{
		MimeType: "image/png",
		Data:     original,
		Source:   "node1",
	})

	got := h.Get()
	if got == nil {
		t.Fatal("Get should return current item")
	}

	got.Data[0] = 9
	original[1] = 8

	again := h.Get()
	if again == nil {
		t.Fatal("Get should still return current item")
	}
	if again.Data[0] != 1 {
		t.Fatalf("Get should not expose internal data slice, got %#v", again.Data)
	}
	if again.Data[1] != 2 {
		t.Fatalf("Put should not retain caller-owned binary slice, got %#v", again.Data)
	}
}

func TestHistoryAndSinceReturnDeepCopies(t *testing.T) {
	h := newTestHub()

	h.Put(PutInput{MimeType: "image/png", Data: []byte{1, 2, 3}, Source: "node1"})
	h.Put(PutInput{MimeType: "image/png", Data: []byte{4, 5, 6}, Source: "node2"})

	hist := h.History(2)
	since := h.Since(1)
	if len(hist) != 2 || len(since) != 1 {
		t.Fatalf("unexpected history=%d since=%d", len(hist), len(since))
	}

	hist[0].Data[0] = 9
	since[0].Data[0] = 8

	againHist := h.History(2)
	againSince := h.Since(1)
	if againHist[0].Data[0] != 4 {
		t.Fatalf("History should not expose internal data slice, got %#v", againHist[0].Data)
	}
	if againSince[0].Data[0] != 4 {
		t.Fatalf("Since should not expose internal data slice, got %#v", againSince[0].Data)
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

func TestPutReleasesStateLockBeforePersistence(t *testing.T) {
	h := newTestHub()
	store := newBlockingStore()
	h.store = store

	putDone := make(chan struct{})
	go func() {
		h.Put(textInput("hello", "node1"))
		close(putDone)
	}()

	saved := <-store.saveStarted
	if saved.Seq != 1 {
		t.Fatalf("expected first persisted seq to be 1, got %d", saved.Seq)
	}

	getDone := make(chan *protocol.ClipItem, 1)
	go func() {
		getDone <- h.Get()
	}()

	select {
	case item := <-getDone:
		if item == nil || item.Content != "hello" {
			t.Fatalf("expected Get to observe the in-memory update, got %+v", item)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Get blocked while persistence was in progress")
	}

	store.allowOneSave()

	select {
	case <-putDone:
	case <-time.After(time.Second):
		t.Fatal("Put did not finish after persistence was released")
	}
}

func TestConcurrentPutsPreserveSubscriberOrder(t *testing.T) {
	h := newTestHub()
	store := newBlockingStore()
	h.store = store

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := h.Subscribe(ctx)

	done := make(chan struct{}, 2)
	go func() {
		h.Put(textInput("first", "node1"))
		done <- struct{}{}
	}()

	firstSaved := <-store.saveStarted
	if firstSaved.Seq != 1 {
		t.Fatalf("expected first persisted seq to be 1, got %d", firstSaved.Seq)
	}

	select {
	case item := <-sub.C:
		if item.Seq != 1 || item.Content != "first" {
			t.Fatalf("expected first subscriber item to be seq 1, got %+v", item)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first subscriber item")
	}

	go func() {
		h.Put(textInput("second", "node2"))
		done <- struct{}{}
	}()

	store.allowOneSave()

	secondSaved := <-store.saveStarted
	if secondSaved.Seq != 2 {
		t.Fatalf("expected second persisted seq to be 2, got %d", secondSaved.Seq)
	}

	select {
	case item := <-sub.C:
		if item.Seq != 2 || item.Content != "second" {
			t.Fatalf("expected second subscriber item to be seq 2, got %+v", item)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for second subscriber item")
	}

	store.allowOneSave()

	for range 2 {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for puts to finish")
		}
	}
}

func TestConcurrentSubscribersReceiveOrderedUpdates(t *testing.T) {
	h := newTestHub()
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	sub1 := h.Subscribe(ctx1)
	sub2 := h.Subscribe(ctx2)

	h.Put(textInput("first", "node1"))
	h.Put(textInput("second", "node2"))

	readTwo := func(t *testing.T, sub *Subscriber) []uint64 {
		t.Helper()
		seqs := make([]uint64, 0, 2)
		for len(seqs) < 2 {
			select {
			case item := <-sub.C:
				seqs = append(seqs, item.Seq)
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for subscriber updates")
			}
		}
		return seqs
	}

	if seqs := readTwo(t, sub1); seqs[0] != 1 || seqs[1] != 2 {
		t.Fatalf("subscriber 1 saw out-of-order seqs %v", seqs)
	}
	if seqs := readTwo(t, sub2); seqs[0] != 1 || seqs[1] != 2 {
		t.Fatalf("subscriber 2 saw out-of-order seqs %v", seqs)
	}
}

func TestReapExpired(t *testing.T) {
	h := &Hub{
		maxHistory: 50,
		ttl:        time.Millisecond,
		subs:       make(map[*Subscriber]struct{}),
		startedAt:  time.Now(),
	}
	h.publishCond = sync.NewCond(&h.publishMu)

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
