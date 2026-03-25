package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

func devIdentity(r *http.Request) string { return "test-node" }

func setupServer(t *testing.T) (*Hub, *httptest.Server) {
	t.Helper()
	h := newTestHub()
	mux := http.NewServeMux()
	Register(mux, h, devIdentity)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return h, srv
}

func TestPostAndGetClip(t *testing.T) {
	_, srv := setupServer(t)

	body := `{"content":"hello world"}`
	resp, err := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var item protocol.ClipItem
	json.NewDecoder(resp.Body).Decode(&item)
	resp.Body.Close()

	if item.Seq != 1 || item.Content != "hello world" || item.MimeType != "text/plain" {
		t.Fatal("unexpected item", item)
	}

	resp, err = http.Get(srv.URL + "/api/clip")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var got protocol.ClipItem
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()

	if got.Content != "hello world" {
		t.Fatal("GET returned wrong content")
	}
}

func TestPostHTML(t *testing.T) {
	_, srv := setupServer(t)

	body := `{"content":"<b>bold</b>","mime_type":"text/html"}`
	resp, err := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var item protocol.ClipItem
	json.NewDecoder(resp.Body).Decode(&item)
	resp.Body.Close()

	if item.MimeType != "text/html" || item.Content != "<b>bold</b>" {
		t.Fatalf("unexpected: mime=%s content=%s", item.MimeType, item.Content)
	}
}

func TestPostBinary(t *testing.T) {
	_, srv := setupServer(t)

	// JSON with base64-encoded data.
	payload := map[string]any{
		"data":      []byte{0x89, 0x50, 0x4e, 0x47},
		"mime_type": "image/png",
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var item protocol.ClipItem
	json.NewDecoder(resp.Body).Decode(&item)
	resp.Body.Close()

	if item.MimeType != "image/png" || len(item.Data) != 4 {
		t.Fatalf("unexpected: mime=%s data_len=%d", item.MimeType, len(item.Data))
	}
}

func TestGetClipEmpty(t *testing.T) {
	_, srv := setupServer(t)

	resp, err := http.Get(srv.URL + "/api/clip")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestPostClipDedup(t *testing.T) {
	_, srv := setupServer(t)

	body := `{"content":"same"}`
	http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))

	resp, _ := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dedup should return 200, got %d", resp.StatusCode)
	}
}

func TestPostClipEmptyContent(t *testing.T) {
	_, srv := setupServer(t)

	resp, _ := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(`{"content":""}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty content, got %d", resp.StatusCode)
	}
}

func TestHistory(t *testing.T) {
	_, srv := setupServer(t)

	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(map[string]string{"content": string(rune('a' + i))})
		http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBuffer(body))
	}

	resp, _ := http.Get(srv.URL + "/api/clip/history?limit=2")
	var items []protocol.ClipItem
	json.NewDecoder(resp.Body).Decode(&items)
	resp.Body.Close()

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Seq != 3 {
		t.Fatalf("expected most recent first, got seq %d", items[0].Seq)
	}
}

func TestClearClip(t *testing.T) {
	_, srv := setupServer(t)

	for _, content := range []string{"first", "second"} {
		body, _ := json.Marshal(map[string]string{"content": content})
		resp, err := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/clip", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/api/clip")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected empty current clip after clear, got %d", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/api/clip/history")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var items []protocol.ClipItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty history after clear, got %+v", items)
	}
}

func TestStatus(t *testing.T) {
	_, srv := setupServer(t)

	resp, _ := http.Get(srv.URL + "/api/status")
	var status map[string]any
	json.NewDecoder(resp.Body).Decode(&status)
	resp.Body.Close()

	if _, ok := status["uptime"]; !ok {
		t.Fatal("missing uptime")
	}
}

func TestWebSocketStream(t *testing.T) {
	_, srv := setupServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, srv.URL+"/api/clip/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	body := `{"content":"ws test"}`
	http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))

	var msg protocol.WSMessage
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "clip_update" || msg.Item.Content != "ws test" {
		t.Fatalf("unexpected ws message: %+v", msg)
	}
}

func TestWebSocketStreamReplaySinceSeq(t *testing.T) {
	_, srv := setupServer(t)

	post := func(body string) {
		t.Helper()
		resp, err := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	post(`{"content":"first"}`)
	post(`{"content":"second"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, srv.URL+"/api/clip/stream?since_seq=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	var replay protocol.WSMessage
	if err := wsjson.Read(ctx, conn, &replay); err != nil {
		t.Fatal(err)
	}
	if replay.Type != "clip_update" || replay.Item == nil || replay.Item.Seq != 2 || replay.Item.Content != "second" {
		t.Fatalf("unexpected replay message: %+v", replay)
	}

	post(`{"content":"third"}`)

	var live protocol.WSMessage
	if err := wsjson.Read(ctx, conn, &live); err != nil {
		t.Fatal(err)
	}
	if live.Type != "clip_update" || live.Item == nil || live.Item.Seq != 3 || live.Item.Content != "third" {
		t.Fatalf("unexpected live message after replay: %+v", live)
	}
}
