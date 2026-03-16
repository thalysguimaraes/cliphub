package discover

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// HubURL attempts to find the cliphub node on the tailnet by running
// "tailscale status --json" and looking for a peer whose hostname is "cliphub".
// Returns the HTTPS URL or an error if not found.
func HubURL() (string, error) {
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

	// Check if we are the hub ourselves.
	if status.Self.HostName == "cliphub" {
		dns := strings.TrimSuffix(status.Self.DNSName, ".")
		return "https://" + dns, nil
	}

	// Look through peers.
	for _, peer := range status.Peer {
		if peer.HostName == "cliphub" {
			dns := strings.TrimSuffix(peer.DNSName, ".")
			return "https://" + dns, nil
		}
	}

	return "", fmt.Errorf("no 'cliphub' node found on tailnet")
}

// SelfName returns this node's tailscale hostname, or falls back to os.Hostname.
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
