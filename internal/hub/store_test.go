package hub

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/thalys/cliphub/internal/protocol"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now()
	item := protocol.ClipItem{
		Seq:       42,
		MimeType:  "text/plain",
		Content:   "hello",
		Hash:      protocol.HashContent("hello"),
		Source:    "node1",
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}

	if err := s.SaveItem(item); err != nil {
		t.Fatal(err)
	}

	seq, items, err := s.LoadState(50)
	if err != nil {
		t.Fatal(err)
	}
	if seq != 42 {
		t.Fatalf("expected seq 42, got %d", seq)
	}
	if len(items) != 1 || items[0].Content != "hello" {
		t.Fatalf("expected 1 item with content 'hello', got %+v", items)
	}
}

func TestStoreBinaryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now()
	data := []byte{0x89, 0x50, 0x4e, 0x47}
	item := protocol.ClipItem{
		Seq:       1,
		MimeType:  "image/png",
		Data:      data,
		Hash:      protocol.HashBytes(data),
		Source:    "node1",
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}

	if err := s.SaveItem(item); err != nil {
		t.Fatal(err)
	}

	_, items, err := s.LoadState(50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].Data) != 4 {
		t.Fatalf("expected 1 binary item, got %+v", items)
	}
}

func TestStoreDeleteExpired(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now()
	s.SaveItem(protocol.ClipItem{
		Seq: 1, MimeType: "text/plain", Content: "expired", Hash: "a",
		Source: "n", CreatedAt: now, ExpiresAt: now.Add(-time.Hour),
	})
	s.SaveItem(protocol.ClipItem{
		Seq: 2, MimeType: "text/plain", Content: "alive", Hash: "b",
		Source: "n", CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	})

	n, err := s.DeleteExpired(now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 deleted, got %d", n)
	}

	_, items, _ := s.LoadState(50)
	if len(items) != 1 || items[0].Content != "alive" {
		t.Fatal("wrong items after delete")
	}
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, _ := OpenStore(dbPath)
	s.SaveItem(protocol.ClipItem{
		Seq: 10, MimeType: "text/plain", Content: "persisted", Hash: "x",
		Source: "n", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	})
	s.Close()

	s2, _ := OpenStore(dbPath)
	defer s2.Close()
	seq, items, _ := s2.LoadState(50)

	if seq != 10 {
		t.Fatalf("expected seq 10 after reopen, got %d", seq)
	}
	if len(items) != 1 || items[0].Content != "persisted" {
		t.Fatal("data not persisted across reopen")
	}
}

func TestSince(t *testing.T) {
	h := newTestHub()

	for i := 0; i < 5; i++ {
		h.Put(textInput(string(rune('a'+i)), "node1"))
	}

	items := h.Since(3)
	if len(items) != 2 {
		t.Fatalf("expected 2 items since seq 3, got %d", len(items))
	}
	// Should be chronological: seq 4 then seq 5.
	if items[0].Seq != 4 || items[1].Seq != 5 {
		t.Fatalf("expected seqs [4,5], got [%d,%d]", items[0].Seq, items[1].Seq)
	}
}

func TestSinceEmpty(t *testing.T) {
	h := newTestHub()
	h.Put(textInput("a", "node1"))

	items := h.Since(1)
	if len(items) != 0 {
		t.Fatalf("expected 0 items since current seq, got %d", len(items))
	}
}

func TestHubWithPersistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	h, err := New(Config{MaxHistory: 10, TTL: time.Hour, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	h.Put(PutInput{MimeType: "text/plain", Content: "survive restart", Source: "n"})
	h.Close()

	// "Restart" the hub.
	h2, err := New(Config{MaxHistory: 10, TTL: time.Hour, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer h2.Close()

	// Wait briefly for write-behind goroutine from first hub.
	time.Sleep(50 * time.Millisecond)

	if h2.Seq() != 1 {
		t.Fatalf("expected seq 1 after restart, got %d", h2.Seq())
	}
	item := h2.Get()
	if item == nil || item.Content != "survive restart" {
		t.Fatal("item not recovered after restart")
	}
}
