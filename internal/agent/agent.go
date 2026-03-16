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

	"github.com/thalys/cliphub/internal/clipboard"
	"github.com/thalys/cliphub/internal/protocol"
)

// Config holds agent configuration.
type Config struct {
	HubURL       string              // Base URL of the hub.
	NodeName     string              // This node's name.
	PollInterval time.Duration       // Clipboard poll interval.
	Clipboard    clipboard.Clipboard // Clipboard backend (nil = system default).
}

// Agent is the local clipboard sync agent.
type Agent struct {
	hubURL       string
	nodeName     string
	pollInterval time.Duration
	monitor      *ClipboardMonitor
	client       *http.Client
	paused       atomic.Bool
	bootstrapped atomic.Bool
}

// New creates an Agent.
func New(cfg Config) *Agent {
	clip := cfg.Clipboard
	if clip == nil {
		c, err := clipboard.New()
		if err != nil {
			slog.Error("clipboard init failed", "err", err)
			os.Exit(1)
		}
		clip = c
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
			if !a.bootstrapped.Load() {
				continue
			}
			result, ct := a.monitor.Poll()
			if result == PollNewContent {
				if err := a.sendToHub(ctx, ct); err != nil {
					slog.Error("failed to send clip to hub", "err", err)
				}
			}
		}
	}
}

func (a *Agent) bootstrap(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, "GET", a.hubURL+"/api/clip", nil)
	if err != nil {
		slog.Error("bootstrap: build request failed", "err", err)
		a.bootstrapped.Store(true)
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
	slog.Info("bootstrap: applied hub clip", "seq", item.Seq, "source", item.Source, "mime", item.MimeType)
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

	ct := itemToContent(item)
	if err := a.monitor.ApplyRemote(ct); err != nil {
		slog.Error("failed to apply remote clip", "err", err)
	} else {
		slog.Info("applied remote clip", "seq", item.Seq, "source", item.Source, "mime", item.MimeType)
	}
}

func (a *Agent) sendToHub(ctx context.Context, ct clipboard.Content) error {
	payload := map[string]any{"mime_type": ct.MimeType}
	if ct.IsText() {
		payload["content"] = ct.Text()
	} else {
		payload["data"] = ct.Data
	}

	body, _ := json.Marshal(payload)
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

	slog.Info("sent clip to hub", "mime", ct.MimeType, "len", len(ct.Data))
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

func itemToContent(item protocol.ClipItem) clipboard.Content {
	if item.IsText() {
		return clipboard.Content{MimeType: item.MimeType, Data: []byte(item.Content)}
	}
	return clipboard.Content{MimeType: item.MimeType, Data: item.Data}
}
