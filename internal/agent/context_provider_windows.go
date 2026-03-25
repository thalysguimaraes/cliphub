//go:build windows

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
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class Win32 {
	[DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
	[DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint pid);
}
"@
$hwnd = [Win32]::GetForegroundWindow()
$pid = 0
[Win32]::GetWindowThreadProcessId($hwnd, [ref]$pid) | Out-Null
if ($pid -eq 0) { exit 1 }
$proc = Get-Process -Id $pid
Write-Output $proc.ProcessName
Write-Output $proc.Path
`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
	if err != nil {
		return privacy.Context{}, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	ctx := privacy.Context{}
	if len(lines) > 0 {
		ctx.ProcessName = strings.TrimSpace(lines[0])
		ctx.AppName = ctx.ProcessName
	}
	if len(lines) > 1 {
		ctx.BundleID = strings.TrimSpace(lines[1])
	}
	return ctx, nil
}
