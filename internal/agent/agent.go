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
	// Start WebSocket listener.
	wsURL := a.hubURL + "/api/clip/stream"
	// Replace http(s) with ws(s) for WebSocket.
	if len(wsURL) > 4 && wsURL[:5] == "https" {
		wsURL = "wss" + wsURL[5:]
	} else if len(wsURL) > 3 && wsURL[:4] == "http" {
		wsURL = "ws" + wsURL[4:]
	}

	ws := &WSClient{
		URL: wsURL,
		OnUpdate: func(item protocol.ClipItem) {
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
		},
	}

	go ws.Run(ctx)

	// Clipboard poll loop.
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
			result, content := a.monitor.Poll()
			if result == PollNewContent {
				if err := a.sendToHub(ctx, content); err != nil {
					slog.Error("failed to send clip to hub", "err", err)
				}
			}
		}
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
