package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/clipboard"
	"github.com/thalysguimaraes/cliphub/internal/hub"
	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

// TestFailedSendRetry is a regression test for the exact failed-send replay
// path. It verifies that when the hub is unreachable during POST /api/clip,
// the agent retries the same clipboard content on subsequent poll ticks
// instead of silently dropping it.
func TestFailedSendRetry(t *testing.T) {
	// Track items the hub receives.
	var received []protocol.ClipItem
	var rejectPosts atomic.Bool

	h, _ := hub.New(hub.Config{MaxHistory: 10, TTL: time.Hour})
	mux := http.NewServeMux()
	hub.Register(mux, h, func(r *http.Request) string {
		return r.Header.Get("X-Clip-Source")
	})

	// Wrap the mux to intercept POSTs and optionally reject them.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/clip" && rejectPosts.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		mux.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	clip := &fakeClipboard{content: clipboard.Content{}}
	a := New(Config{
		HubURL:       srv.URL,
		NodeName:     "test",
		PollInterval: 50 * time.Millisecond,
		Clipboard:    clip,
	})
	// Skip bootstrap (hub is empty).
	a.bootstrapped.Store(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go a.Run(ctx)

	// 1. Hub is rejecting posts. Set clipboard content.
	rejectPosts.Store(true)
	clip.content = textContent("important-data")

	// Wait for a few poll cycles — the agent should try and fail.
	time.Sleep(200 * time.Millisecond)

	// Hub should have nothing.
	if item := h.Get(); item != nil {
		t.Fatal("hub should not have received anything while rejecting")
	}

	// 2. Bring hub back up.
	rejectPosts.Store(false)

	// Wait for the agent to retry.
	time.Sleep(200 * time.Millisecond)

	// Hub should now have the item.
	item := h.Get()
	if item == nil {
		t.Fatal("hub should have received the item after recovery")
	}
	if item.Content != "important-data" {
		t.Fatalf("expected 'important-data', got %q", item.Content)
	}

	received = append(received, *item)
	_ = received

	// 3. Verify the item is not sent again (no infinite retry after success).
	seq := h.Seq()
	time.Sleep(200 * time.Millisecond)
	if h.Seq() != seq {
		t.Fatal("item should not be re-sent after successful delivery")
	}
}

// TestFailedSendNewContentOverrides verifies that if the user copies new
// content while a send is failing, the newer content is what gets sent
// when the hub comes back (not the stale failed item).
func TestFailedSendNewContentOverrides(t *testing.T) {
	var rejectPosts atomic.Bool

	h, _ := hub.New(hub.Config{MaxHistory: 10, TTL: time.Hour})
	mux := http.NewServeMux()
	hub.Register(mux, h, func(r *http.Request) string {
		return r.Header.Get("X-Clip-Source")
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/clip" && rejectPosts.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		mux.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	clip := &fakeClipboard{content: clipboard.Content{}}
	a := New(Config{
		HubURL:       srv.URL,
		NodeName:     "test",
		PollInterval: 50 * time.Millisecond,
		Clipboard:    clip,
	})
	a.bootstrapped.Store(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go a.Run(ctx)

	// Hub is down. User copies "old".
	rejectPosts.Store(true)
	clip.content = textContent("old")
	time.Sleep(200 * time.Millisecond)

	// User copies "new" while hub is still down.
	clip.content = textContent("new")
	time.Sleep(200 * time.Millisecond)

	// Bring hub back.
	rejectPosts.Store(false)
	time.Sleep(200 * time.Millisecond)

	item := h.Get()
	if item == nil {
		t.Fatal("hub should have received an item")
	}
	// The hub should have "new", not "old".
	if item.Content != "new" {
		t.Fatalf("expected 'new', got %q", item.Content)
	}
}

// TestBootstrapPreventsStaleOverwrite verifies that the agent does not
// send stale local clipboard content before bootstrapping from the hub.
func TestBootstrapPreventsStaleOverwrite(t *testing.T) {
	h, _ := hub.New(hub.Config{MaxHistory: 10, TTL: time.Hour})
	mux := http.NewServeMux()
	hub.Register(mux, h, func(r *http.Request) string {
		return r.Header.Get("X-Clip-Source")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Pre-populate hub with a clip from another node.
	h.Put(hub.PutInput{
		MimeType: "text/plain",
		Content:  "hub-content",
		Source:   "other-node",
	})

	// Local clipboard has stale content.
	clip := &fakeClipboard{content: textContent("stale-local")}
	a := New(Config{
		HubURL:       srv.URL,
		NodeName:     "test",
		PollInterval: 50 * time.Millisecond,
		Clipboard:    clip,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go a.Run(ctx)
	time.Sleep(500 * time.Millisecond)

	// The hub should still have "hub-content", not "stale-local".
	item := h.Get()
	if item == nil || item.Content != "hub-content" {
		var got string
		if item != nil {
			got = item.Content
		}
		t.Fatalf("expected hub to keep 'hub-content', got %q", got)
	}

	// The local clipboard should have been updated to "hub-content".
	local := clip.content
	if local.Text() != "hub-content" {
		t.Fatalf("expected local clipboard to be 'hub-content', got %q", local.Text())
	}
}

func TestMIMETypeChangeDetected(t *testing.T) {
	h, _ := hub.New(hub.Config{MaxHistory: 10, TTL: time.Hour})
	mux := http.NewServeMux()
	hub.Register(mux, h, func(r *http.Request) string {
		return r.Header.Get("X-Clip-Source")
	})
	srv := httptest.NewServer(handler(mux))
	defer srv.Close()

	clip := &fakeClipboard{content: clipboard.Content{
		MimeType: "text/plain",
		Data:     []byte("hello"),
	}}
	a := New(Config{
		HubURL:       srv.URL,
		NodeName:     "test",
		PollInterval: 50 * time.Millisecond,
		Clipboard:    clip,
	})
	a.bootstrapped.Store(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Run(ctx)

	time.Sleep(200 * time.Millisecond)

	item := h.Get()
	if item == nil || item.MimeType != "text/plain" {
		t.Fatal("expected text/plain item")
	}
	seq1 := h.Seq()

	// Same bytes, different MIME type.
	clip.content = clipboard.Content{
		MimeType: "text/html",
		Data:     []byte("hello"),
	}
	time.Sleep(200 * time.Millisecond)

	if h.Seq() <= seq1 {
		t.Fatal("MIME type change should produce a new item")
	}
	item = h.Get()
	if item.MimeType != "text/html" {
		t.Fatalf("expected text/html, got %s", item.MimeType)
	}
}

func handler(mux *http.ServeMux) http.Handler {
	return mux
}

func init() {
	// Suppress noisy logs during tests.
	_ = json.Unmarshal
}
