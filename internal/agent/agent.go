package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/clipboard"
	"github.com/thalysguimaraes/cliphub/internal/hubclient"
	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

// Config holds agent configuration.
type Config struct {
	HubURL       string              // Base URL of the hub.
	Client       *hubclient.Client   // Shared hub client (preferred when set).
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
	client       *hubclient.Client
	paused       atomic.Bool
	bootstrapped atomic.Bool
}

// ClipboardInitError reports a failure to initialize the default clipboard backend.
type ClipboardInitError struct {
	Err error
}

func (e *ClipboardInitError) Error() string {
	if e == nil || e.Err == nil {
		return "clipboard init failed"
	}
	return e.Err.Error()
}

func (e *ClipboardInitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

var newClipboard = clipboard.New

// New creates an Agent.
func New(cfg Config) (*Agent, error) {
	clip := cfg.Clipboard
	if clip == nil {
		c, err := newClipboard()
		if err != nil {
			return nil, &ClipboardInitError{Err: err}
		}
		clip = c
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}

	client := cfg.Client
	if client == nil && cfg.HubURL != "" {
		var err error
		client, err = hubclient.New(hubclient.Config{BaseURL: cfg.HubURL})
		if err != nil {
			return nil, err
		}
	}
	if client != nil {
		cfg.HubURL = client.BaseURL()
	}

	return &Agent{
		hubURL:       cfg.HubURL,
		nodeName:     cfg.NodeName,
		pollInterval: cfg.PollInterval,
		monitor:      NewClipboardMonitor(clip),
		client:       client,
	}, nil
}

// Run starts the clipboard poll loop and WebSocket listener.
func (a *Agent) Run(ctx context.Context) error {
	if a.client == nil {
		return fmt.Errorf("hub client is not configured")
	}

	ws := &WSClient{
		URL: a.client.StreamURL(),
		OnConnected: func() {
			if !a.bootstrapped.Load() {
				a.bootstrap(ctx)
			}
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
			if a.isPaused() {
				continue
			}
			if !a.bootstrapped.Load() {
				continue
			}
			result, ct := a.monitor.Poll()
			if result == PollNewContent {
				if err := a.sendToHub(ctx, ct); err != nil {
					slog.Error("failed to send clip to hub, will retry", "err", err)
				} else {
					a.monitor.MarkSent()
				}
			}
		}
	}
}

func (a *Agent) bootstrap(ctx context.Context) {
	backoff := 500 * time.Millisecond
	const maxRetries = 5

	for attempt := 1; attempt <= maxRetries; attempt++ {
		ok := a.tryBootstrap(ctx)
		if ok {
			return
		}
		if ctx.Err() != nil {
			a.bootstrapped.Store(true)
			return
		}
		slog.Warn("bootstrap: retry", "attempt", attempt, "retry_in", backoff)
		select {
		case <-ctx.Done():
			a.bootstrapped.Store(true)
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 5*time.Second)
	}

	slog.Error("bootstrap: all retries exhausted, proceeding without hub state")
	a.bootstrapped.Store(true)
}

// tryBootstrap attempts a single bootstrap fetch. Returns true on success.
func (a *Agent) tryBootstrap(ctx context.Context) bool {
	item, err := a.client.Current(ctx)
	if errors.Is(err, hubclient.ErrNoCurrentClip) {
		slog.Info("bootstrap: hub has no current clip")
		a.bootstrapped.Store(true)
		return true
	}
	if err != nil {
		slog.Warn("bootstrap: fetch failed", "err", err)
		return false
	}

	a.applyRemote(*item)
	slog.Info("bootstrap: applied hub clip", "seq", item.Seq, "source", item.Source, "mime", item.MimeType)
	a.bootstrapped.Store(true)
	return true
}

func (a *Agent) applyRemote(item protocol.ClipItem) {
	if a.isPaused() {
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

func (a *Agent) isPaused() bool {
	return a.paused.Load() || a.isPausedByFile()
}

func (a *Agent) sendToHub(ctx context.Context, ct clipboard.Content) error {
	payload := hubclient.PutRequest{MimeType: ct.MimeType, Source: a.nodeName}
	if ct.IsText() {
		payload.Content = ct.Text()
	} else {
		payload.Data = ct.Data
	}

	if _, err := a.client.Put(ctx, payload); err != nil {
		return err
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
