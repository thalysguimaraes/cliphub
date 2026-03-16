//go:build windows

package clipboard

import (
	"bytes"
	"os/exec"
)

type windowsClipboard struct{}

// New returns a Clipboard for Windows (text/plain only for now).
func New() (Clipboard, error) {
	return &windowsClipboard{}, nil
}

func (c *windowsClipboard) ReadBest() (Content, error) {
	out, err := exec.Command("powershell", "-command", "Get-Clipboard -Raw").Output()
	if err != nil {
		return Content{}, err
	}
	return Content{MimeType: "text/plain", Data: out}, nil
}

func (c *windowsClipboard) Write(ct Content) error {
	// Windows only supports text/plain via PowerShell for now.
	cmd := exec.Command("powershell", "-command", "Set-Clipboard -Value $input")
	cmd.Stdin = bytes.NewReader(ct.Data)
	return cmd.Run()
}
