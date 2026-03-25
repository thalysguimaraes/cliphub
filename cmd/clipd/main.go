package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/agent"
	"github.com/thalysguimaraes/cliphub/internal/discover"
	"github.com/thalysguimaraes/cliphub/internal/hubclient"
	"github.com/thalysguimaraes/cliphub/internal/privacy"
)

// version is injected via ldflags in reproducible release builds.
var version = "dev"

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
	ignoreApps := fs.String("ignore-apps", envString("CLIPHUB_IGNORE_APPS", ""), "comma-separated app names or bundle IDs to keep local")
	ignoreProcesses := fs.String("ignore-processes", envString("CLIPHUB_IGNORE_PROCESSES", ""), "comma-separated process names to keep local")
	filterSensitive := fs.String("filter-sensitive", envString("CLIPHUB_FILTER_SENSITIVE", ""), "comma-separated sensitive classes to block (secret,password-manager,otp)")
	clearOnBlock := fs.Bool("clear-on-block", envBool("CLIPHUB_CLEAR_ON_BLOCK", false), "clear the local clipboard when a privacy rule blocks sync")
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

	sensitiveClasses, err := privacy.ParseSensitiveClasses(*filterSensitive)
	if err != nil {
		return err
	}

	a, err := newAgent(agent.Config{
		HubURL:       *hubURL,
		Client:       client,
		NodeName:     *nodeName,
		PollInterval: time.Duration(*pollMs) * time.Millisecond,
		Privacy: privacy.NewConfig(
			privacy.ParseCSV(*ignoreApps),
			privacy.ParseCSV(*ignoreProcesses),
			sensitiveClasses,
			*clearOnBlock,
		),
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

func envBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func envString(key string, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
