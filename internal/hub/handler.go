package hub

import (
	"context"
	"encoding/json"
	"fmt"
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
func Register(mux *http.ServeMux, h *Hub, identFn IdentityFunc, observers ...*Observer) {
	obs := NewObserver()
	if len(observers) > 0 && observers[0] != nil {
		obs = observers[0]
	}

	handle := func(pattern string, route string, handler http.HandlerFunc) {
		mux.Handle(pattern, withObservability(obs, route, handler))
	}

	handle("POST /api/clip", "/api/clip", postClipHandler(h, identFn, obs))
	handle("GET /api/clip", "/api/clip", getClipHandler(h))
	handle("GET /api/clip/history", "/api/clip/history", historyHandler(h))
	handle("GET /api/clip/stream", "/api/clip/stream", streamHandler(h, obs))
	handle("GET /api/status", "/api/status", statusHandler(h, obs))
	handle("GET /healthz", "/healthz", healthHandler(h, obs))
	handle("GET /readyz", "/readyz", readinessHandler(obs))
	handle("GET /metrics", "/metrics", metricsHandler(h, obs))
}

type postClipRequest struct {
	Content  string `json:"content,omitempty"`
	Data     []byte `json:"data,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

func postClipHandler(h *Hub, identFn IdentityFunc, obs *Observer) http.HandlerFunc {
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
			obs.RecordClipStored()
		} else {
			obs.RecordClipDeduplicated()
		}
		json.NewEncoder(w).Encode(item)

		if isNew {
			slog.Info(
				"clip stored",
				"component", "hub_api",
				"request_id", requestIDFromContext(r.Context()),
				"sequence", item.Seq,
				"source", source,
				"mime_type", item.MimeType,
				"payload_bytes", len(item.RawBytes()),
			)
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

func streamHandler(h *Hub, obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			slog.Error(
				"websocket accept failed",
				"component", "hub_stream",
				"request_id", requestIDFromContext(r.Context()),
				"error", err,
			)
			return
		}
		defer conn.CloseNow()
		obs.RecordWSConnect()
		defer obs.RecordWSDisconnect()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		go func() {
			select {
			case <-ctx.Done():
			case <-obs.ShutdownContext().Done():
				cancel()
			}
		}()

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
						slog.Debug(
							"websocket catch-up write failed",
							"component", "hub_stream",
							"request_id", requestIDFromContext(r.Context()),
							"error", err,
						)
						return
					}
					if item.Seq > replayedUpTo {
						replayedUpTo = item.Seq
					}
				}
				if len(missed) > 0 {
					obs.RecordWSCatchup(len(missed))
					slog.Info(
						"websocket catch-up replay",
						"component", "hub_stream",
						"request_id", requestIDFromContext(r.Context()),
						"since_sequence", sinceSeq,
						"replayed_items", len(missed),
					)
				}
			}
		}

		slog.Info(
			"subscriber connected",
			"component", "hub_stream",
			"request_id", requestIDFromContext(r.Context()),
			"remote_addr", r.RemoteAddr,
		)

		for {
			select {
			case <-ctx.Done():
				_ = conn.Close(websocket.StatusNormalClosure, "bye")
				return
			case item := <-sub.C:
				if item.Seq <= replayedUpTo {
					continue // Already sent during catch-up.
				}
				msg := protocol.WSMessage{Type: "clip_update", Item: &item}
				if err := wsjson.Write(ctx, conn, msg); err != nil {
					slog.Debug(
						"websocket write failed",
						"component", "hub_stream",
						"request_id", requestIDFromContext(r.Context()),
						"error", err,
					)
					return
				}
			}
		}
	}
}

func statusHandler(h *Hub, obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, obs.Status(h))
	}
}

func healthHandler(h *Hub, obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"ready":  obs.Ready(),
			"uptime": time.Since(h.StartedAt()).String(),
		})
	}
}

func readinessHandler(obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statusCode := http.StatusOK
		status := "ready"
		if !obs.Ready() {
			statusCode = http.StatusServiceUnavailable
			status = "shutting_down"
		}
		writeJSON(w, statusCode, map[string]any{
			"status": status,
			"ready":  obs.Ready(),
		})
	}
}

func metricsHandler(h *Hub, obs *Observer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprint(w, obs.Metrics(h))
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
