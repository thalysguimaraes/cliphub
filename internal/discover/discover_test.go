package discover

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHubHostnameFromEnv(t *testing.T) {
	t.Setenv("CLIPHUB_HOSTNAME", "")
	if got := HubHostnameFromEnv(); got != DefaultHubHostname {
		t.Fatalf("expected default hostname %q, got %q", DefaultHubHostname, got)
	}

	t.Setenv("CLIPHUB_HOSTNAME", "custom-hub")
	if got := HubHostnameFromEnv(); got != "custom-hub" {
		t.Fatalf("expected env hostname override, got %q", got)
	}
}

func TestResolverCachesStatusAcrossCalls(t *testing.T) {
	var (
		now        = time.Unix(100, 0)
		readCalls  int
		probeCalls int
	)

	resolver := NewResolver(Config{
		HubHostname: "custom-hub",
		CacheTTL:    time.Minute,
		readStatus: func(context.Context) (tailnetStatus, error) {
			readCalls++
			return tailnetStatus{
				Self: tailnetNode{HostName: "laptop", DNSName: "laptop.tailnet.ts.net."},
				Peer: map[string]tailnetNode{
					"peer": {HostName: "custom-hub", DNSName: "custom-hub.tailnet.ts.net."},
				},
			}, nil
		},
		probeURL: func(context.Context, string) (string, error) {
			probeCalls++
			return "https://custom-hub.tailnet.ts.net", nil
		},
		now: func() time.Time { return now },
	})

	hubURL, err := resolver.HubURL(context.Background())
	if err != nil {
		t.Fatalf("HubURL() error = %v", err)
	}
	if hubURL != "https://custom-hub.tailnet.ts.net" {
		t.Fatalf("unexpected hub URL %q", hubURL)
	}

	selfName, err := resolver.SelfName(context.Background())
	if err != nil {
		t.Fatalf("SelfName() error = %v", err)
	}
	if selfName != "laptop" {
		t.Fatalf("unexpected self name %q", selfName)
	}

	hubURL, err = resolver.HubURL(context.Background())
	if err != nil {
		t.Fatalf("HubURL() second call error = %v", err)
	}
	if hubURL != "https://custom-hub.tailnet.ts.net" {
		t.Fatalf("unexpected cached hub URL %q", hubURL)
	}

	if readCalls != 1 {
		t.Fatalf("expected one tailscale status read, got %d", readCalls)
	}
	if probeCalls != 1 {
		t.Fatalf("expected one hub probe, got %d", probeCalls)
	}
}

func TestResolverHubURLMissingDiscoveryData(t *testing.T) {
	resolver := NewResolver(Config{
		HubHostname: "custom-hub",
		readStatus: func(context.Context) (tailnetStatus, error) {
			return tailnetStatus{
				Self: tailnetNode{HostName: "laptop", DNSName: "laptop.tailnet.ts.net."},
			}, nil
		},
		probeURL: func(context.Context, string) (string, error) {
			t.Fatal("probeURL should not run when no matching hub exists")
			return "", nil
		},
	})

	_, err := resolver.HubURL(context.Background())
	if err == nil {
		t.Fatal("expected missing hub error")
	}
	if !strings.Contains(err.Error(), `no "custom-hub" node found on tailnet`) {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestResolverSelfNameMissingHostname(t *testing.T) {
	resolver := NewResolver(Config{
		readStatus: func(context.Context) (tailnetStatus, error) {
			return tailnetStatus{}, nil
		},
	})

	_, err := resolver.SelfName(context.Background())
	if err == nil {
		t.Fatal("expected missing hostname error")
	}
	if !strings.Contains(err.Error(), "empty hostname") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestReadTailnetStatusCommandFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := readTailnetStatus(ctx)
	if err == nil {
		t.Fatal("expected command failure")
	}
}

func TestDefaultConfigUsesEnvOverride(t *testing.T) {
	t.Setenv("CLIPHUB_HOSTNAME", "ci-hub")
	cfg := DefaultConfig()
	if cfg.HubHostname != "ci-hub" {
		t.Fatalf("expected env hostname in config, got %q", cfg.HubHostname)
	}
	if cfg.CacheTTL <= 0 {
		t.Fatalf("expected positive cache TTL, got %s", cfg.CacheTTL)
	}
	if cfg.ProbeTimeout <= 0 {
		t.Fatalf("expected positive probe timeout, got %s", cfg.ProbeTimeout)
	}
}

func TestHubURLLegacyHelperUsesEnv(t *testing.T) {
	t.Setenv("CLIPHUB_HOSTNAME", "legacy-hub")

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	defer t.Setenv("PATH", origPath)

	if _, err := HubURL(); err == nil {
		t.Fatal("expected helper to attempt tailscale lookup")
	}
}
