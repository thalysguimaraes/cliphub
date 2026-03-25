//go:build darwin

package agent

import (
	"os/exec"
	"strings"

	"github.com/thalysguimaraes/cliphub/internal/privacy"
)

type shellContextProvider struct{}

func newContextProvider() contextProvider {
	return shellContextProvider{}
}

func (shellContextProvider) CurrentContext() (privacy.Context, error) {
	script := `
tell application "System Events"
	set frontApp to first application process whose frontmost is true
	set appName to name of frontApp
	try
		set bundleID to bundle identifier of frontApp
	on error
		set bundleID to ""
	end try
	return appName & linefeed & bundleID
end tell
`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return privacy.Context{}, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	ctx := privacy.Context{}
	if len(lines) > 0 {
		ctx.AppName = strings.TrimSpace(lines[0])
		ctx.ProcessName = ctx.AppName
	}
	if len(lines) > 1 {
		ctx.BundleID = strings.TrimSpace(lines[1])
	}
	return ctx, nil
}
