package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/clipboard"
	"github.com/thalysguimaraes/cliphub/internal/hub"
	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

func TestNewReturnsClipboardInitError(t *testing.T) {
	wantErr := errors.New("clipboard unavailable")
	origNewClipboard := newClipboard
	newClipboard = func() (clipboard.Clipboard, error) {
		return nil, wantErr
	}
	t.Cleanup(func() {
		newClipboard = origNewClipboard
	})

	a, err := New(Config{HubURL: "http://example.com", NodeName: "test"})
	if err == nil {
		t.Fatal("expected clipboard init error")
	}
	if a != nil {
		t.Fatal("expected agent to be nil on init failure")
	}

	var initErr *ClipboardInitError
	if !errors.As(err, &initErr) {
		t.Fatalf("expected ClipboardInitError, got %T", err)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped error %v, got %v", wantErr, err)
	}
}

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
	a, err := New(Config{
		HubURL:       srv.URL,
		NodeName:     "test",
		PollInterval: 50 * time.Millisecond,
		Clipboard:    clip,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
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
	a, err := New(Config{
		HubURL:       srv.URL,
		NodeName:     "test",
		PollInterval: 50 * time.Millisecond,
		Clipboard:    clip,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
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
	a, err := New(Config{
		HubURL:       srv.URL,
		NodeName:     "test",
		PollInterval: 50 * time.Millisecond,
		Clipboard:    clip,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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
	a, err := New(Config{
		HubURL:       srv.URL,
		NodeName:     "test",
		PollInterval: 50 * time.Millisecond,
		Clipboard:    clip,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
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

func TestPauseSourcesBlockRemoteApplyUntilResumed(t *testing.T) {
	tests := []struct {
		name  string
		pause func(t *testing.T, a *Agent) func()
	}{
		{
			name: "in-memory pause",
			pause: func(t *testing.T, a *Agent) func() {
				a.paused.Store(true)
				return func() { a.paused.Store(false) }
			},
		},
		{
			name: "pause file",
			pause: func(t *testing.T, a *Agent) func() {
				home := t.TempDir()
				t.Setenv("HOME", home)
				pausedPath := filepath.Join(home, ".config", "cliphub", "paused")
				if err := os.MkdirAll(filepath.Dir(pausedPath), 0o755); err != nil {
					t.Fatalf("mkdir pause dir: %v", err)
				}
				if err := os.WriteFile(pausedPath, []byte("paused\n"), 0o644); err != nil {
					t.Fatalf("write pause file: %v", err)
				}
				return func() {
					if err := os.Remove(pausedPath); err != nil && !os.IsNotExist(err) {
						t.Fatalf("remove pause file: %v", err)
					}
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clip := &fakeClipboard{content: clipboard.Content{}}
			a, err := New(Config{NodeName: "test", Clipboard: clip})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			resume := tc.pause(t, a)
			a.applyRemote(protocol.ClipItem{
				MimeType: "text/plain",
				Content:  "blocked-remote",
				Source:   "other-node",
			})
			if got := clip.content.Text(); got != "" {
				t.Fatalf("expected paused remote apply to be blocked, got %q", got)
			}

			resume()
			a.applyRemote(protocol.ClipItem{
				MimeType: "text/plain",
				Content:  "after-resume",
				Source:   "other-node",
			})
			if got := clip.content.Text(); got != "after-resume" {
				t.Fatalf("expected remote apply after resume, got %q", got)
			}
		})
	}
}

func TestPauseSourcesBlockLocalCaptureUntilResumed(t *testing.T) {
	tests := []struct {
		name  string
		pause func(t *testing.T, a *Agent) func()
	}{
		{
			name: "in-memory pause",
			pause: func(t *testing.T, a *Agent) func() {
				a.paused.Store(true)
				return func() { a.paused.Store(false) }
			},
		},
		{
			name: "pause file",
			pause: func(t *testing.T, a *Agent) func() {
				home := t.TempDir()
				t.Setenv("HOME", home)
				pausedPath := filepath.Join(home, ".config", "cliphub", "paused")
				if err := os.MkdirAll(filepath.Dir(pausedPath), 0o755); err != nil {
					t.Fatalf("mkdir pause dir: %v", err)
				}
				if err := os.WriteFile(pausedPath, []byte("paused\n"), 0o644); err != nil {
					t.Fatalf("write pause file: %v", err)
				}
				return func() {
					if err := os.Remove(pausedPath); err != nil && !os.IsNotExist(err) {
						t.Fatalf("remove pause file: %v", err)
					}
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := hub.New(hub.Config{MaxHistory: 10, TTL: time.Hour})
			mux := http.NewServeMux()
			hub.Register(mux, h, func(r *http.Request) string {
				return r.Header.Get("X-Clip-Source")
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()

			clip := &fakeClipboard{content: textContent("local-while-paused")}
			a, err := New(Config{
				HubURL:       srv.URL,
				NodeName:     "test",
				PollInterval: 20 * time.Millisecond,
				Clipboard:    clip,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			a.bootstrapped.Store(true)

			resume := tc.pause(t, a)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go a.Run(ctx)

			time.Sleep(120 * time.Millisecond)
			if item := h.Get(); item != nil {
				t.Fatalf("expected no local capture while paused, got %q", item.Content)
			}

			resume()
			waitFor(t, 500*time.Millisecond, func() bool {
				item := h.Get()
				return item != nil && item.Content == "local-while-paused"
			})
		})
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("condition not satisfied before timeout")
}
