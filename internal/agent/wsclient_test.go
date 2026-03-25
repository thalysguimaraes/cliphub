package agent

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/hub"
	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

func TestWSClientReconnectsAndCatchesUpAfterDrop(t *testing.T) {
	h, _ := hub.New(hub.Config{MaxHistory: 10, TTL: time.Hour})
	mux := http.NewServeMux()
	hub.Register(mux, h, func(r *http.Request) string {
		return r.Header.Get("X-Clip-Source")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	streamURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/clip/stream"

	var (
		mu       sync.Mutex
		received []uint64
	)
	connected := make(chan struct{}, 2)
	firstUpdate := make(chan struct{})
	allUpdates := make(chan struct{})

	client := &WSClient{
		URL: streamURL,
		OnConnected: func() {
			select {
			case connected <- struct{}{}:
			default:
			}
		},
		OnUpdate: func(item protocol.ClipItem) {
			mu.Lock()
			received = append(received, item.Seq)
			count := len(received)
			mu.Unlock()

			if count == 1 {
				close(firstUpdate)
			}
			if count == 3 {
				close(allUpdates)
			}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	go client.Run(ctx)

	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket connection")
	}

	post := func(content string) {
		t.Helper()
		body := `{"content":"` + content + `"}`
		resp, err := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	post("first")

	select {
	case <-firstUpdate:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first websocket update")
	}

	srv.CloseClientConnections()
	time.Sleep(150 * time.Millisecond)

	post("second")
	post("third")

	select {
	case <-allUpdates:
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for reconnect catch-up")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 3 {
		t.Fatalf("expected 3 updates, got %v", received)
	}
	for i, seq := range []uint64{1, 2, 3} {
		if received[i] != seq {
			t.Fatalf("expected reconnect sequence [1 2 3], got %v", received)
		}
	}
}
