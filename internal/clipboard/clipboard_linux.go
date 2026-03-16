//go:build linux

package clipboard

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type linuxClipboard struct {
	backend string // "wayland" or "x11"
}

// New returns a Clipboard for the current Linux display server.
func New() (Clipboard, error) {
	if _, err := exec.LookPath("wl-paste"); err == nil {
		return &linuxClipboard{backend: "wayland"}, nil
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		return &linuxClipboard{backend: "x11"}, nil
	}
	return nil, fmt.Errorf("no clipboard tool found (need wl-paste/wl-copy or xclip)")
}

func (c *linuxClipboard) ReadBest() (Content, error) {
	types, err := c.listTypes()
	if err != nil {
		// Fall back to plain text read.
		return c.readType("text/plain")
	}

	best := bestType(types)
	if best == "" {
		return Content{}, nil
	}
	return c.readType(best)
}

func (c *linuxClipboard) Write(ct Content) error {
	switch c.backend {
	case "wayland":
		return c.wlWrite(ct)
	default:
		return c.xclipWrite(ct)
	}
}

func (c *linuxClipboard) listTypes() ([]string, error) {
	var cmd *exec.Cmd
	switch c.backend {
	case "wayland":
		cmd = exec.Command("wl-paste", "--list-types")
	default:
		cmd = exec.Command("xclip", "-selection", "clipboard", "-t", "TARGETS", "-o")
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var types []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			types = append(types, line)
		}
	}
	return types, nil
}

func (c *linuxClipboard) readType(mimeType string) (Content, error) {
	var cmd *exec.Cmd
	switch c.backend {
	case "wayland":
		cmd = exec.Command("wl-paste", "--no-newline", "--type", mimeType)
	default:
		cmd = exec.Command("xclip", "-selection", "clipboard", "-t", mimeType, "-o")
	}
	out, err := cmd.Output()
	if err != nil {
		return Content{}, err
	}
	return Content{MimeType: mimeType, Data: out}, nil
}

func (c *linuxClipboard) wlWrite(ct Content) error {
	cmd := exec.Command("wl-copy", "--type", ct.MimeType)
	cmd.Stdin = bytes.NewReader(ct.Data)
	return cmd.Run()
}

func (c *linuxClipboard) xclipWrite(ct Content) error {
	cmd := exec.Command("xclip", "-selection", "clipboard", "-t", ct.MimeType, "-i")
	cmd.Stdin = bytes.NewReader(ct.Data)
	return cmd.Run()
}
