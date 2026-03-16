package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/thalys/cliphub/internal/agent"
)

func main() {
	hubURL := flag.String("hub", envOr("CLIPHUB_HUB", "http://localhost:8080"), "hub URL")
	nodeName := flag.String("node", hostname(), "this node's name")
	pollMs := flag.Int("poll", 500, "clipboard poll interval in milliseconds")
	flag.Parse()

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

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
