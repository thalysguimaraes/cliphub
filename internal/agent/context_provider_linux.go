//go:build linux

package agent

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/thalysguimaraes/cliphub/internal/privacy"
)

type shellContextProvider struct{}

func newContextProvider() contextProvider {
	return shellContextProvider{}
}

func (shellContextProvider) CurrentContext() (privacy.Context, error) {
	if _, err := exec.LookPath("xdotool"); err != nil {
		return privacy.Context{}, fmt.Errorf("xdotool not found: %w", err)
	}

	pidOut, err := exec.Command("xdotool", "getactivewindow", "getwindowpid").Output()
	if err != nil {
		return privacy.Context{}, err
	}
	pid := strings.TrimSpace(string(pidOut))
	if pid == "" {
		return privacy.Context{}, fmt.Errorf("active window pid is empty")
	}

	commOut, err := exec.Command("ps", "-p", pid, "-o", "comm=").Output()
	if err != nil {
		return privacy.Context{}, err
	}
	processName := strings.TrimSpace(string(commOut))

	argsOut, err := exec.Command("ps", "-p", pid, "-o", "args=").Output()
	if err != nil {
		return privacy.Context{}, err
	}
	appName := strings.TrimSpace(string(argsOut))
	if appName == "" {
		appName = processName
	}

	return privacy.Context{
		AppName:     appName,
		ProcessName: processName,
	}, nil
}
