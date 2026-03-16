package discover

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// HubURL attempts to find the cliphub node on the tailnet by running
// "tailscale status --json" and looking for a peer whose hostname is "cliphub".
// Probes HTTPS first, falls back to HTTP.
func HubURL() (string, error) {
	dns, err := findHubDNS()
	if err != nil {
		return "", err
	}

	// Probe HTTPS (port 443), then HTTP (port 80).
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}

	httpsURL := "https://" + dns
	if resp, err := client.Get(httpsURL + "/api/status"); err == nil {
		resp.Body.Close()
		return httpsURL, nil
	}

	httpURL := "http://" + dns
	if resp, err := client.Get(httpURL + "/api/status"); err == nil {
		resp.Body.Close()
		return httpURL, nil
	}

	// Hub found on tailnet but not responding; return HTTPS as default.
	return httpsURL, nil
}

func findHubDNS() (string, error) {
	out, err := exec.Command("tailscale", "status", "--json").Output()
	if err != nil {
		return "", fmt.Errorf("tailscale status: %w", err)
	}

	var status struct {
		Peer map[string]struct {
			HostName string `json:"HostName"`
			DNSName  string `json:"DNSName"`
		} `json:"Peer"`
		Self struct {
			HostName string `json:"HostName"`
			DNSName  string `json:"DNSName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return "", fmt.Errorf("parse tailscale status: %w", err)
	}

	if status.Self.HostName == "cliphub" {
		return strings.TrimSuffix(status.Self.DNSName, "."), nil
	}

	for _, peer := range status.Peer {
		if peer.HostName == "cliphub" {
			return strings.TrimSuffix(peer.DNSName, "."), nil
		}
	}

	return "", fmt.Errorf("no 'cliphub' node found on tailnet")
}

// SelfName returns this node's tailscale hostname.
func SelfName() (string, error) {
	out, err := exec.Command("tailscale", "status", "--json").Output()
	if err != nil {
		return "", err
	}

	var status struct {
		Self struct {
			HostName string `json:"HostName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return "", err
	}
	if status.Self.HostName != "" {
		return status.Self.HostName, nil
	}
	return "", fmt.Errorf("empty hostname in tailscale status")
}
