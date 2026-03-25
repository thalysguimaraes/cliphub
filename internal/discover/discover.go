package discover

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultHubHostname preserves the historic tailnet hostname.
	DefaultHubHostname  = "cliphub"
	defaultCacheTTL     = 30 * time.Second
	defaultProbeTimeout = 3 * time.Second
)

// Config controls discovery behavior.
type Config struct {
	HubHostname  string
	CacheTTL     time.Duration
	ProbeTimeout time.Duration

	readStatus func(context.Context) (tailnetStatus, error)
	probeURL   func(context.Context, string) (string, error)
	now        func() time.Time
}

// Resolver caches tailscale discovery results for a short time so repeated
// calls within a process do not shell out over and over.
type Resolver struct {
	hubHostname string
	cacheTTL    time.Duration
	readStatus  func(context.Context) (tailnetStatus, error)
	probeURL    func(context.Context, string) (string, error)
	now         func() time.Time

	mu    sync.Mutex
	cache resolverCache
}

type resolverCache struct {
	status    tailnetStatus
	hubURL    string
	expiresAt time.Time
	valid     bool
}

type tailnetStatus struct {
	Peer map[string]tailnetNode `json:"Peer"`
	Self tailnetNode            `json:"Self"`
}

type tailnetNode struct {
	HostName string `json:"HostName"`
	DNSName  string `json:"DNSName"`
}

// DefaultConfig returns discovery defaults using the current environment.
func DefaultConfig() Config {
	return Config{
		HubHostname:  HubHostnameFromEnv(),
		CacheTTL:     defaultCacheTTL,
		ProbeTimeout: defaultProbeTimeout,
	}
}

// HubHostnameFromEnv returns the configured tailnet hostname for the hub.
func HubHostnameFromEnv() string {
	if value := strings.TrimSpace(os.Getenv("CLIPHUB_HOSTNAME")); value != "" {
		return value
	}
	return DefaultHubHostname
}

// NewResolver constructs a resolver with caching, context-aware status lookups,
// and hub probing.
func NewResolver(cfg Config) *Resolver {
	if cfg.HubHostname == "" {
		cfg.HubHostname = DefaultHubHostname
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = defaultCacheTTL
	}
	if cfg.ProbeTimeout <= 0 {
		cfg.ProbeTimeout = defaultProbeTimeout
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	if cfg.readStatus == nil {
		cfg.readStatus = readTailnetStatus
	}

	resolver := &Resolver{
		hubHostname: cfg.HubHostname,
		cacheTTL:    cfg.CacheTTL,
		readStatus:  cfg.readStatus,
		now:         cfg.now,
	}
	if cfg.probeURL != nil {
		resolver.probeURL = cfg.probeURL
	} else {
		resolver.probeURL = func(ctx context.Context, dns string) (string, error) {
			return probeHubURL(ctx, dns, cfg.ProbeTimeout)
		}
	}

	return resolver
}

// HubURL resolves the hub base URL using the configured tailnet hostname.
func (r *Resolver) HubURL(ctx context.Context) (string, error) {
	status, cachedURL, err := r.status(ctx)
	if err != nil {
		return "", err
	}
	if cachedURL != "" {
		return cachedURL, nil
	}

	dns, err := r.findHubDNS(status)
	if err != nil {
		return "", err
	}

	hubURL, err := r.probeURL(ctx, dns)
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	if r.cache.valid && r.now().Before(r.cache.expiresAt) {
		r.cache.hubURL = hubURL
	}
	r.mu.Unlock()

	return hubURL, nil
}

// SelfName returns this node's tailscale hostname.
func (r *Resolver) SelfName(ctx context.Context) (string, error) {
	status, _, err := r.status(ctx)
	if err != nil {
		return "", err
	}
	if status.Self.HostName == "" {
		return "", fmt.Errorf("empty hostname in tailscale status")
	}
	return status.Self.HostName, nil
}

// HubURL preserves the legacy convenience helper.
func HubURL() (string, error) {
	return NewResolver(DefaultConfig()).HubURL(context.Background())
}

// SelfName preserves the legacy convenience helper.
func SelfName() (string, error) {
	return NewResolver(DefaultConfig()).SelfName(context.Background())
}

func (r *Resolver) status(ctx context.Context) (tailnetStatus, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	r.mu.Lock()
	if r.cache.valid && r.now().Before(r.cache.expiresAt) {
		status := r.cache.status
		hubURL := r.cache.hubURL
		r.mu.Unlock()
		return status, hubURL, nil
	}
	r.mu.Unlock()

	status, err := r.readStatus(ctx)
	if err != nil {
		return tailnetStatus{}, "", err
	}

	r.mu.Lock()
	r.cache = resolverCache{
		status:    status,
		expiresAt: r.now().Add(r.cacheTTL),
		valid:     true,
	}
	r.mu.Unlock()

	return status, "", nil
}

func (r *Resolver) findHubDNS(status tailnetStatus) (string, error) {
	if status.Self.HostName == r.hubHostname {
		return trimDNS(status.Self.DNSName), nil
	}

	for _, peer := range status.Peer {
		if peer.HostName == r.hubHostname {
			return trimDNS(peer.DNSName), nil
		}
	}

	return "", fmt.Errorf("no %q node found on tailnet", r.hubHostname)
}

func readTailnetStatus(ctx context.Context) (tailnetStatus, error) {
	out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
	if err != nil {
		return tailnetStatus{}, fmt.Errorf("tailscale status: %w", err)
	}

	var status tailnetStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return tailnetStatus{}, fmt.Errorf("parse tailscale status: %w", err)
	}

	return status, nil
}

func probeHubURL(ctx context.Context, dns string, timeout time.Duration) (string, error) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}

	httpsURL := "https://" + dns
	if err := probeStatusEndpoint(ctx, client, httpsURL, timeout); err == nil {
		return httpsURL, nil
	}

	httpURL := "http://" + dns
	if err := probeStatusEndpoint(ctx, client, httpURL, timeout); err == nil {
		return httpURL, nil
	}

	// Preserve the existing bias toward HTTPS when the host exists but the probe
	// cannot complete successfully.
	return httpsURL, nil
}

func probeStatusEndpoint(ctx context.Context, client *http.Client, baseURL string, timeout time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint, err := url.JoinPath(baseURL, "api", "status")
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func trimDNS(dns string) string {
	return strings.TrimSuffix(dns, ".")
}
