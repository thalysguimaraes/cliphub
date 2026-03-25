package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/discover"
	"github.com/thalysguimaraes/cliphub/internal/hub"
	"tailscale.com/tsnet"
)

func main() {
	dev := flag.Bool("dev", false, "development mode: listen on localhost without tsnet")
	addr := flag.String("addr", "localhost:8080", "listen address in dev mode")
	hostname := flag.String("hostname", envString("CLIPHUB_HOSTNAME", discover.DefaultHubHostname), "tailnet hostname in tsnet mode")
	stateDir := flag.String("state-dir", defaultStateDir(), "tsnet state directory")
	maxHistory := flag.Int("max-history", envInt("CLIPHUB_MAX_HISTORY", 50), "max history items")
	ttl := flag.Duration("ttl", envDuration("CLIPHUB_TTL", 24*time.Hour), "item TTL")
	flag.Parse()

	dbPath := ""
	if !*dev {
		if err := os.MkdirAll(*stateDir, 0o700); err != nil {
			slog.Error("create state dir failed", "component", "cliphub", "error", err, "state_dir", *stateDir)
			os.Exit(1)
		}
		dbPath = filepath.Join(*stateDir, "clips.db")
	}

	h, err := hub.New(hub.Config{
		MaxHistory: *maxHistory,
		TTL:        *ttl,
		DBPath:     dbPath,
	})
	if err != nil {
		slog.Error("hub init failed", "component", "cliphub", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := h.Close(); err != nil {
			slog.Error("hub close failed", "component", "cliphub", "error", err)
		}
	}()

	mux := http.NewServeMux()
	obs := hub.NewObserver()

	var ln net.Listener

	if *dev {
		hub.Register(mux, h, func(r *http.Request) string {
			if name := r.Header.Get("X-Clip-Source"); name != "" {
				return name
			}
			return "dev"
		}, obs)

		ln, err = net.Listen("tcp", *addr)
		if err != nil {
			slog.Error("listen failed", "component", "cliphub", "error", err, "listen_addr", *addr)
			os.Exit(1)
		}
		slog.Info("cliphub dev mode", "component", "cliphub", "listen_addr", *addr)
	} else {
		srv := &tsnet.Server{
			Hostname: *hostname,
			Dir:      *stateDir,
		}
		defer srv.Close()

		// Try TLS first (requires HTTPS enabled in Tailscale admin).
		// Fall back to plain HTTP if HTTPS is not configured.
		ln, err = srv.ListenTLS("tcp", ":443")
		if err != nil {
			slog.Warn("TLS listen failed; falling back to plain HTTP", "component", "cliphub", "error", err)
			ln, err = srv.Listen("tcp", ":80")
			if err != nil {
				slog.Error("tsnet listen failed", "component", "cliphub", "error", err)
				os.Exit(1)
			}
			slog.Info("cliphub listening on tailnet (plain HTTP)", "component", "cliphub", "hostname", *hostname)
		}

		lc, err := srv.LocalClient()
		if err != nil {
			slog.Error("tsnet local client failed", "component", "cliphub", "error", err)
			os.Exit(1)
		}

		hub.Register(mux, h, func(r *http.Request) string {
			who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
			if err != nil {
				return "unknown"
			}
			return who.Node.ComputedName
		}, obs)

		slog.Info("cliphub listening on tailnet", "component", "cliphub", "hostname", *hostname, "state_dir", *stateDir)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.Serve(ln)
	}()

	select {
	case sig := <-sigCh:
		obs.BeginShutdown()
		slog.Info("shutdown requested", "component", "cliphub", "signal", sig.String())

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("graceful shutdown failed", "component", "cliphub", "error", err)
			if closeErr := server.Close(); closeErr != nil {
				slog.Error("server close failed", "component", "cliphub", "error", closeErr)
			}
			os.Exit(1)
		}

		if err := <-serveErrCh; err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "component", "cliphub", "error", err)
			os.Exit(1)
		}
	case err := <-serveErrCh:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "component", "cliphub", "error", err)
			os.Exit(1)
		}
	}
}

func defaultStateDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "cliphub", "tsnet")
	}
	return "/var/lib/cliphub/tsnet"
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
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
