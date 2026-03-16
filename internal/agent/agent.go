package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/thalys/cliphub/internal/protocol"
)

// Config holds agent configuration.
type Config struct {
	HubURL       string        // Base URL of the hub (e.g. https://cliphub.tailnet.ts.net)
	NodeName     string        // This node's name.
	PollInterval time.Duration // Clipboard poll interval.
	Clipboard    Clipboard     // Clipboard backend (nil = system).
}

// Agent is the local clipboard sync agent.
type Agent struct {
	hubURL       string
	nodeName     string
	pollInterval time.Duration
	monitor      *ClipboardMonitor
	client       *http.Client
	paused       atomic.Bool
	bootstrapped atomic.Bool // True after first successful bootstrap.
}

// New creates an Agent.
func New(cfg Config) *Agent {
	clip := cfg.Clipboard
	if clip == nil {
		clip = SystemClipboard{}
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	return &Agent{
		hubURL:       cfg.HubURL,
		nodeName:     cfg.NodeName,
		pollInterval: cfg.PollInterval,
		monitor:      NewClipboardMonitor(clip),
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

// Run starts the clipboard poll loop and WebSocket listener.
// Blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	wsURL := a.hubURL + "/api/clip/stream"
	if len(wsURL) > 4 && wsURL[:5] == "https" {
		wsURL = "wss" + wsURL[5:]
	} else if len(wsURL) > 3 && wsURL[:4] == "http" {
		wsURL = "ws" + wsURL[4:]
	}

	ws := &WSClient{
		URL: wsURL,
		OnConnected: func() {
			// Fetch current clip from hub to avoid clobbering newer hub state
			// with stale local clipboard on startup or after reconnect.
			a.bootstrap(ctx)
		},
		OnUpdate: func(item protocol.ClipItem) {
			a.applyRemote(item)
		},
	}

	go ws.Run(ctx)

	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	slog.Info("clipd started", "hub", a.hubURL, "node", a.nodeName, "poll", a.pollInterval)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if a.paused.Load() || a.isPausedByFile() {
				continue
			}
			// Don't send local clips until we've bootstrapped from the hub,
			// otherwise a stale local clipboard overwrites newer hub state.
			if !a.bootstrapped.Load() {
				continue
			}
			result, content := a.monitor.Poll()
			if result == PollNewContent {
				if err := a.sendToHub(ctx, content); err != nil {
					slog.Error("failed to send clip to hub", "err", err)
				}
			}
		}
	}
}

// bootstrap fetches the current clip from the hub and applies it locally.
// This ensures we don't clobber newer hub state with stale local content.
func (a *Agent) bootstrap(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, "GET", a.hubURL+"/api/clip", nil)
	if err != nil {
		slog.Error("bootstrap: build request failed", "err", err)
		a.bootstrapped.Store(true) // Don't block forever on failure.
		return
	}

	resp, err := a.client.Do(req)
	if err != nil {
		slog.Warn("bootstrap: fetch failed, will sync from local clipboard", "err", err)
		a.bootstrapped.Store(true)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		slog.Info("bootstrap: hub has no current clip")
		a.bootstrapped.Store(true)
		return
	}

	var item protocol.ClipItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		slog.Warn("bootstrap: decode failed", "err", err)
		a.bootstrapped.Store(true)
		return
	}

	a.applyRemote(item)
	slog.Info("bootstrap: applied hub clip", "seq", item.Seq, "source", item.Source)
	a.bootstrapped.Store(true)
}

func (a *Agent) applyRemote(item protocol.ClipItem) {
	if a.paused.Load() {
		return
	}
	if item.Source == a.nodeName {
		slog.Debug("ignoring own update", "seq", item.Seq)
		return
	}
	if err := a.monitor.ApplyRemote(item.Content); err != nil {
		slog.Error("failed to apply remote clip", "err", err)
	} else {
		slog.Info("applied remote clip", "seq", item.Seq, "source", item.Source)
	}
}

func (a *Agent) sendToHub(ctx context.Context, content string) error {
	body, _ := json.Marshal(map[string]string{"content": content})
	req, err := http.NewRequestWithContext(ctx, "POST", a.hubURL+"/api/clip", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Clip-Source", a.nodeName)

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("hub returned %d", resp.StatusCode)
	}

	slog.Info("sent clip to hub", "len", len(content))
	return nil
}

func (a *Agent) isPausedByFile() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".config", "cliphub", "paused"))
	return err == nil
}
