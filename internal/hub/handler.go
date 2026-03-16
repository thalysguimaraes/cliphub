package hub

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/thalys/cliphub/internal/protocol"
)

// IdentityFunc extracts a node/source name from an HTTP request.
// In production this uses Tailscale WhoIs; in dev mode it can be a no-op.
type IdentityFunc func(r *http.Request) string

// Register mounts all API routes on mux.
func Register(mux *http.ServeMux, h *Hub, identFn IdentityFunc) {
	mux.HandleFunc("POST /api/clip", postClipHandler(h, identFn))
	mux.HandleFunc("GET /api/clip", getClipHandler(h))
	mux.HandleFunc("GET /api/clip/history", historyHandler(h))
	mux.HandleFunc("GET /api/clip/stream", streamHandler(h))
	mux.HandleFunc("GET /api/status", statusHandler(h))
}

type postClipRequest struct {
	Content string `json:"content"`
}

func postClipHandler(h *Hub, identFn IdentityFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, int64(protocol.MaxContentSize)+1))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		if len(body) > protocol.MaxContentSize {
			http.Error(w, "content too large", http.StatusRequestEntityTooLarge)
			return
		}

		var req postClipRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Content == "" {
			http.Error(w, "empty content", http.StatusBadRequest)
			return
		}

		source := identFn(r)
		item, isNew := h.Put(req.Content, source)

		w.Header().Set("Content-Type", "application/json")
		if isNew {
			w.WriteHeader(http.StatusCreated)
		}
		json.NewEncoder(w).Encode(item)

		if isNew {
			slog.Info("clip stored", "seq", item.Seq, "source", source, "len", len(req.Content))
		}
	}
}

func getClipHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item := h.Get()
		if item == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(item)
	}
}

func historyHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if s := r.URL.Query().Get("limit"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				limit = n
			}
		}
		items := h.History(limit)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}
}

func streamHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			slog.Error("websocket accept failed", "err", err)
			return
		}
		defer conn.CloseNow()

		ctx := r.Context()
		sub := h.Subscribe(ctx)
		defer sub.cancel()

		slog.Info("subscriber connected", "remote", r.RemoteAddr)

		for {
			select {
			case <-ctx.Done():
				conn.Close(websocket.StatusNormalClosure, "bye")
				return
			case item := <-sub.C:
				msg := protocol.WSMessage{Type: "clip_update", Item: &item}
				if err := wsjson.Write(ctx, conn, msg); err != nil {
					slog.Debug("websocket write failed", "err", err)
					return
				}
			}
		}
	}
}

func statusHandler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := map[string]any{
			"uptime":      time.Since(h.StartedAt()).String(),
			"seq":         h.Seq(),
			"subscribers": h.SubscriberCount(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}
}
