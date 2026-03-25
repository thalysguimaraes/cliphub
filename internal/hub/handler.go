package hub

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

// IdentityFunc extracts a node/source name from an HTTP request.
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
	Content  string `json:"content,omitempty"`
	Data     []byte `json:"data,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
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

		// Default to text/plain for backward compatibility.
		if req.MimeType == "" {
			req.MimeType = "text/plain"
		}

		if strings.HasPrefix(req.MimeType, "text/") {
			if req.Content == "" {
				http.Error(w, "empty content", http.StatusBadRequest)
				return
			}
		} else {
			if len(req.Data) == 0 {
				http.Error(w, "empty data", http.StatusBadRequest)
				return
			}
		}

		source := identFn(r)
		item, isNew := h.Put(PutInput{
			MimeType: req.MimeType,
			Content:  req.Content,
			Data:     req.Data,
			Source:   source,
		})

		w.Header().Set("Content-Type", "application/json")
		if isNew {
			w.WriteHeader(http.StatusCreated)
		}
		json.NewEncoder(w).Encode(item)

		if isNew {
			slog.Info("clip stored", "seq", item.Seq, "source", source, "mime", item.MimeType, "len", len(item.RawBytes()))
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

		// Subscribe first so we don't miss items arriving during catch-up.
		sub := h.Subscribe(ctx)
		defer sub.cancel()

		// Catch-up replay: send items missed since the given seq.
		var replayedUpTo uint64
		if s := r.URL.Query().Get("since_seq"); s != "" {
			if sinceSeq, err := strconv.ParseUint(s, 10, 64); err == nil {
				missed := h.Since(sinceSeq)
				for _, item := range missed {
					msg := protocol.WSMessage{Type: "clip_update", Item: &item}
					if err := wsjson.Write(ctx, conn, msg); err != nil {
						slog.Debug("websocket catch-up write failed", "err", err)
						return
					}
					if item.Seq > replayedUpTo {
						replayedUpTo = item.Seq
					}
				}
				if len(missed) > 0 {
					slog.Info("catch-up replay", "since_seq", sinceSeq, "replayed", len(missed))
				}
			}
		}

		slog.Info("subscriber connected", "remote", r.RemoteAddr)

		for {
			select {
			case <-ctx.Done():
				conn.Close(websocket.StatusNormalClosure, "bye")
				return
			case item := <-sub.C:
				if item.Seq <= replayedUpTo {
					continue // Already sent during catch-up.
				}
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
