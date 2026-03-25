package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/agent"
	"github.com/thalysguimaraes/cliphub/internal/discover"
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
	if *hubURL == "" {
		url, err := discover.HubURL()
		if err != nil {
			slog.Warn("hub auto-discovery failed, falling back to localhost", "err", err)
			*hubURL = "http://localhost:8080"
		} else {
			slog.Info("discovered hub", "url", url)
			*hubURL = url
		}
	}

	if *nodeName == "" {
		name, err := discover.SelfName()
		if err != nil {
			h, _ := os.Hostname()
			*nodeName = h
		} else {
			*nodeName = name
		}
	}

	a, err := newAgent(agent.Config{
		HubURL:       *hubURL,
		NodeName:     *nodeName,
		PollInterval: time.Duration(*pollMs) * time.Millisecond,
	})
	if err != nil {
		return err
	}

	return a.Run(ctx)
}

func runMain(args []string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx, args); err != nil && err != context.Canceled {
		var initErr *agent.ClipboardInitError
		if errors.As(err, &initErr) {
			slog.Error("clipboard init failed", "err", initErr.Err)
		} else {
			slog.Error("clipd exited", "err", err)
		}
		return 1
	}
	return 0
}

func main() {
	os.Exit(runMain(os.Args[1:]))
}
