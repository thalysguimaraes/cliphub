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
	"github.com/thalysguimaraes/cliphub/internal/privacy"
	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

// Config holds agent configuration.
type Config struct {
	HubURL          string              // Base URL of the hub.
	Client          *hubclient.Client   // Shared hub client (preferred when set).
	NodeName        string              // This node's name.
	PollInterval    time.Duration       // Clipboard poll interval.
	Clipboard       clipboard.Clipboard // Clipboard backend (nil = system default).
	Privacy         privacy.Config      // Optional privacy policy for outbound clips.
	ContextProvider contextProvider     // Optional active app/process detector.
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
	privacy      privacy.Config
	ctxProvider  contextProvider
	warnedCtx    atomic.Bool
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
		privacy:      cfg.Privacy,
		ctxProvider:  resolveContextProvider(cfg),
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

	slog.Info("clipd started", "component", "clipd", "hub_url", a.hubURL, "node_name", a.nodeName, "poll_interval", a.pollInterval)

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
				if blocked := a.handlePrivacy(ct); blocked {
					continue
				}
				if err := a.sendToHub(ctx, ct); err != nil {
					slog.Error("failed to send clip to hub, will retry", "component", "clipd", "error", err)
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
		slog.Warn("bootstrap retry", "component", "clipd_bootstrap", "attempt", attempt, "retry_delay", backoff)
		select {
		case <-ctx.Done():
			a.bootstrapped.Store(true)
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 5*time.Second)
	}

	slog.Error("bootstrap retries exhausted; proceeding without hub state", "component", "clipd_bootstrap")
	a.bootstrapped.Store(true)
}

// tryBootstrap attempts a single bootstrap fetch. Returns true on success.
func (a *Agent) tryBootstrap(ctx context.Context) bool {
	item, err := a.client.Current(ctx)
	if errors.Is(err, hubclient.ErrNoCurrentClip) {
		slog.Info("bootstrap found no current clip", "component", "clipd_bootstrap")
		a.bootstrapped.Store(true)
		return true
	}
	if err != nil {
		slog.Warn("bootstrap fetch failed", "component", "clipd_bootstrap", "error", err)
		return false
	}

	a.applyRemote(*item)
	slog.Info("bootstrap applied hub clip", "component", "clipd_bootstrap", "sequence", item.Seq, "source", item.Source, "mime_type", item.MimeType)
	a.bootstrapped.Store(true)
	return true
}

func (a *Agent) applyRemote(item protocol.ClipItem) {
	if a.isPaused() {
		return
	}
	if item.Source == a.nodeName {
		slog.Debug("ignoring own update", "component", "clipd", "sequence", item.Seq)
		return
	}

	ct := itemToContent(item)
	if err := a.monitor.ApplyRemote(ct); err != nil {
		slog.Error("failed to apply remote clip", "component", "clipd", "error", err)
	} else {
		slog.Info("applied remote clip", "component", "clipd", "sequence", item.Seq, "source", item.Source, "mime_type", item.MimeType)
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

	slog.Info("sent clip to hub", "component", "clipd", "mime_type", ct.MimeType, "payload_bytes", len(ct.Data))
	return nil
}

func (a *Agent) handlePrivacy(ct clipboard.Content) bool {
	if a.privacy.Empty() {
		return false
	}

	ctx := privacy.Context{}
	if a.ctxProvider != nil && a.privacy.UsesContext() {
		detected, err := a.ctxProvider.CurrentContext()
		if err != nil {
			if a.warnedCtx.CompareAndSwap(false, true) {
				slog.Warn("privacy context unavailable; app/process rules will be best-effort", "err", err)
			}
		} else {
			ctx = detected
		}
	}

	decision := a.privacy.Decide(ctx, ct)
	if !decision.Block {
		return false
	}

	if decision.ClearClipboard {
		if err := a.monitor.ClearLocal(); err != nil {
			a.monitor.MarkHandled()
			slog.Warn("privacy rule blocked clip but failed to clear local clipboard", "rule", decision.Rule, "matched", decision.Matched, "err", err)
		}
	} else {
		a.monitor.MarkHandled()
	}

	slog.Info("blocked local clipboard from sync", "rule", decision.Rule, "matched", decision.Matched, "mime", ct.MimeType)
	return true
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

func resolveContextProvider(cfg Config) contextProvider {
	if cfg.ContextProvider != nil {
		return cfg.ContextProvider
	}
	if cfg.Privacy.UsesContext() {
		return newContextProvider()
	}
	return noopContextProvider{}
}
