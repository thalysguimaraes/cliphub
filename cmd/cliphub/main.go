package main

import (
	"crypto/tls"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/thalys/cliphub/internal/hub"
	"tailscale.com/tsnet"
)

func main() {
	dev := flag.Bool("dev", false, "development mode: listen on localhost:8080 without tsnet")
	addr := flag.String("addr", "localhost:8080", "listen address in dev mode")
	maxHistory := flag.Int("max-history", envInt("CLIPHUB_MAX_HISTORY", 50), "max history items")
	ttl := flag.Duration("ttl", envDuration("CLIPHUB_TTL", 24*time.Hour), "item TTL")
	flag.Parse()

	h := hub.New(hub.Config{
		MaxHistory: *maxHistory,
		TTL:        *ttl,
	})

	mux := http.NewServeMux()

	var ln net.Listener
	var err error

	if *dev {
		hub.Register(mux, h, func(r *http.Request) string {
			if name := r.Header.Get("X-Clip-Source"); name != "" {
				return name
			}
			return "dev"
		})

		ln, err = net.Listen("tcp", *addr)
		if err != nil {
			slog.Error("listen failed", "err", err)
			os.Exit(1)
		}
		slog.Info("cliphub dev mode", "addr", *addr)
	} else {
		srv := &tsnet.Server{
			Hostname: "cliphub",
		}
		defer srv.Close()

		ln, err = srv.ListenTLS("tcp", ":443")
		if err != nil {
			slog.Error("tsnet listen failed", "err", err)
			os.Exit(1)
		}

		lc, err := srv.LocalClient()
		if err != nil {
			slog.Error("tsnet local client failed", "err", err)
			os.Exit(1)
		}

		hub.Register(mux, h, func(r *http.Request) string {
			who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
			if err != nil {
				return "unknown"
			}
			return who.Node.ComputedName
		})

		slog.Info("cliphub listening on tailnet", "hostname", "cliphub")
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		slog.Info("shutting down")
		server.Close()
	}()

	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
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
