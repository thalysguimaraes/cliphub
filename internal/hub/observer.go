package hub

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type requestIDKey struct{}

// Observer tracks operator-facing lifecycle and lightweight metrics.
type Observer struct {
	ready atomic.Bool

	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	nextRequestID atomic.Uint64

	httpRequestsTotal      atomic.Uint64
	clipsStoredTotal       atomic.Uint64
	clipsDeduplicatedTotal atomic.Uint64
	wsConnectionsTotal     atomic.Uint64
	wsDisconnectsTotal     atomic.Uint64
	wsCatchupReplaysTotal  atomic.Uint64
	wsCatchupItemsTotal    atomic.Uint64
}

// NewObserver returns a ready lifecycle observer.
func NewObserver() *Observer {
	ctx, cancel := context.WithCancel(context.Background())
	o := &Observer{
		shutdownCtx:    ctx,
		shutdownCancel: cancel,
	}
	o.ready.Store(true)
	return o
}

// BeginShutdown flips readiness and cancels long-lived request contexts.
func (o *Observer) BeginShutdown() {
	if o == nil {
		return
	}
	if o.ready.Swap(false) {
		o.shutdownCancel()
	}
}

func (o *Observer) Ready() bool {
	if o == nil {
		return true
	}
	return o.ready.Load()
}

func (o *Observer) ShutdownContext() context.Context {
	if o == nil {
		return context.Background()
	}
	return o.shutdownCtx
}

func (o *Observer) WithRequestID(ctx context.Context) (context.Context, string) {
	if o == nil {
		return ctx, ""
	}
	id := fmt.Sprintf("req-%06d", o.nextRequestID.Add(1))
	return context.WithValue(ctx, requestIDKey{}, id), id
}

func (o *Observer) RecordHTTPRequest() {
	if o != nil {
		o.httpRequestsTotal.Add(1)
	}
}

func (o *Observer) RecordClipStored() {
	if o != nil {
		o.clipsStoredTotal.Add(1)
	}
}

func (o *Observer) RecordClipDeduplicated() {
	if o != nil {
		o.clipsDeduplicatedTotal.Add(1)
	}
}

func (o *Observer) RecordWSConnect() {
	if o != nil {
		o.wsConnectionsTotal.Add(1)
	}
}

func (o *Observer) RecordWSDisconnect() {
	if o != nil {
		o.wsDisconnectsTotal.Add(1)
	}
}

func (o *Observer) RecordWSCatchup(items int) {
	if o == nil || items <= 0 {
		return
	}
	o.wsCatchupReplaysTotal.Add(1)
	o.wsCatchupItemsTotal.Add(uint64(items))
}

func (o *Observer) Status(h *Hub) map[string]any {
	return map[string]any{
		"status":                   "ok",
		"ready":                    o.Ready(),
		"uptime":                   time.Since(h.StartedAt()).String(),
		"seq":                      h.Seq(),
		"subscribers":              h.SubscriberCount(),
		"clips_stored_total":       o.clipsStoredTotal.Load(),
		"clips_deduplicated_total": o.clipsDeduplicatedTotal.Load(),
		"http_requests_total":      o.httpRequestsTotal.Load(),
		"ws_connections_total":     o.wsConnectionsTotal.Load(),
		"ws_disconnects_total":     o.wsDisconnectsTotal.Load(),
		"ws_catchup_replays_total": o.wsCatchupReplaysTotal.Load(),
		"ws_catchup_items_total":   o.wsCatchupItemsTotal.Load(),
	}
}

func (o *Observer) Metrics(h *Hub) string {
	var b strings.Builder

	writeMetric(&b, "cliphub_ready", "gauge", boolMetric(o.Ready()))
	writeMetric(&b, "cliphub_http_requests_total", "counter", o.httpRequestsTotal.Load())
	writeMetric(&b, "cliphub_clips_stored_total", "counter", o.clipsStoredTotal.Load())
	writeMetric(&b, "cliphub_clips_deduplicated_total", "counter", o.clipsDeduplicatedTotal.Load())
	writeMetric(&b, "cliphub_ws_connections_total", "counter", o.wsConnectionsTotal.Load())
	writeMetric(&b, "cliphub_ws_disconnects_total", "counter", o.wsDisconnectsTotal.Load())
	writeMetric(&b, "cliphub_ws_catchup_replays_total", "counter", o.wsCatchupReplaysTotal.Load())
	writeMetric(&b, "cliphub_ws_catchup_items_total", "counter", o.wsCatchupItemsTotal.Load())
	writeMetric(&b, "cliphub_sequence", "gauge", h.Seq())
	writeMetric(&b, "cliphub_subscribers", "gauge", uint64(h.SubscriberCount()))
	writeMetricFloat(&b, "cliphub_uptime_seconds", "gauge", time.Since(h.StartedAt()).Seconds())

	return b.String()
}

func withObservability(o *Observer, route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		ctx, requestID := o.WithRequestID(r.Context())
		if requestID != "" {
			w.Header().Set("X-Request-ID", requestID)
		}

		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r.WithContext(ctx))

		statusCode := rec.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		o.RecordHTTPRequest()

		slog.Info(
			"http request completed",
			"component", "hub_http",
			"request_id", requestID,
			"route", route,
			"method", r.Method,
			"path", r.URL.Path,
			"status_code", statusCode,
			"duration_ms", time.Since(started).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	return r.ResponseWriter.Write(p)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("http.ResponseWriter does not implement http.Hijacker")
	}
	return hijacker.Hijack()
}

func (r *statusRecorder) ReadFrom(src io.Reader) (int64, error) {
	if readerFrom, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		if r.statusCode == 0 {
			r.statusCode = http.StatusOK
		}
		return readerFrom.ReadFrom(src)
	}
	return io.Copy(r.ResponseWriter, src)
}

func (r *statusRecorder) Push(target string, opts *http.PushOptions) error {
	pusher, ok := r.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func writeMetric(b *strings.Builder, name string, metricType string, value uint64) {
	fmt.Fprintf(b, "# TYPE %s %s\n%s %d\n", name, metricType, name, value)
}

func writeMetricFloat(b *strings.Builder, name string, metricType string, value float64) {
	fmt.Fprintf(b, "# TYPE %s %s\n%s %.6f\n", name, metricType, name, value)
}

func boolMetric(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}
