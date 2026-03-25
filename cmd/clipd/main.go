package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/agent"
	"github.com/thalysguimaraes/cliphub/internal/discover"
)

func main() {
	hubURL := flag.String("hub", "", "hub URL (auto-discovered from tailnet if empty)")
	nodeName := flag.String("node", "", "this node's name (auto-discovered from tailscale if empty)")
	pollMs := flag.Int("poll", 500, "clipboard poll interval in milliseconds")
	flag.Parse()

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

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	a := agent.New(agent.Config{
		HubURL:       *hubURL,
		NodeName:     *nodeName,
		PollInterval: time.Duration(*pollMs) * time.Millisecond,
	})

	if err := a.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("clipd exited", "err", err)
		os.Exit(1)
	}
}
