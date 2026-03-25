package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/agent"
	"github.com/thalysguimaraes/cliphub/internal/discover"
	"github.com/thalysguimaraes/cliphub/internal/hubclient"
)

type agentRunner interface {
	Run(context.Context) error
}

var newAgent = func(cfg agent.Config) (agentRunner, error) {
	return agent.New(cfg)
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("clipd", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	hubURL := fs.String("hub", "", "hub URL (auto-discovered from tailnet if empty)")
	nodeName := fs.String("node", "", "this node's name (auto-discovered from tailscale if empty)")
	pollMs := fs.Int("poll", 500, "clipboard poll interval in milliseconds")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *hubURL == "" {
		*hubURL = os.Getenv("CLIPHUB_HUB")
	}
	resolver := discover.NewResolver(discover.DefaultConfig())
	if *hubURL == "" {
		url, err := resolver.HubURL(ctx)
		if err != nil {
			slog.Warn("hub auto-discovery failed; falling back to localhost", "component", "clipd", "error", err)
			*hubURL = "http://localhost:8080"
		} else {
			slog.Info("discovered hub", "component", "clipd", "hub_url", url)
			*hubURL = url
		}
	}

	if *nodeName == "" {
		name, err := resolver.SelfName(ctx)
		if err != nil {
			h, _ := os.Hostname()
			*nodeName = h
		} else {
			*nodeName = name
		}
	}

	client, err := hubclient.New(hubclient.Config{BaseURL: *hubURL})
	if err != nil {
		return err
	}

	a, err := newAgent(agent.Config{
		HubURL:       *hubURL,
		Client:       client,
		NodeName:     *nodeName,
		PollInterval: time.Duration(*pollMs) * time.Millisecond,
	})
	if err != nil {
		return err
	}

	return a.Run(ctx)
}

func runMain(args []string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, args); err != nil && err != context.Canceled {
		var initErr *agent.ClipboardInitError
		if errors.As(err, &initErr) {
			slog.Error("clipboard init failed", "component", "clipd", "error", initErr.Err)
		} else {
			slog.Error("clipd exited", "component", "clipd", "error", err)
		}
		return 1
	}
	return 0
}

func main() {
	os.Exit(runMain(os.Args[1:]))
}
