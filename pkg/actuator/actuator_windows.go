//go:build windows

package actuator

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// WindowsActuator implements GUI automation on Windows using PowerShell / Win32.
type WindowsActuator struct {
	mu     sync.Mutex
	dryRun bool
}

// NewWindowsActuator creates a new Windows message actuator.
func NewWindowsActuator() *WindowsActuator {
	return &WindowsActuator{}
}

// NewActuator creates an OS-appropriate message actuator for Windows.
func NewActuator() Actuator {
	return NewWindowsActuator()
}

func (a *WindowsActuator) SetDryRun(dryRun bool) {
	a.dryRun = dryRun
}

func (a *WindowsActuator) IsDryRun() bool {
	return a.dryRun
}

func (a *WindowsActuator) SendMessage(ctx context.Context, recipient, content string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(recipient) == "" {
		return ErrEmptyRecipient
	}
	if strings.TrimSpace(content) == "" {
		return ErrEmptyContent
	}
	if a.dryRun {
		return nil
	}

	// PowerShell automation for Windows WeChat:
	// 1. Activate WeChat window ("微信" or "WeChat")
	// 2. Ctrl+F search recipient, Enter
	// 3. Paste content (via clipboard), Enter to send
	psScript := fmt.Sprintf(`
$wshell = New-Object -ComObject WScript.Shell
$activated = $wshell.AppActivate("微信")
if (-not $activated) {
    $activated = $wshell.AppActivate("WeChat")
}
if (-not $activated) {
    throw "WeChat window not found"
}
Start-Sleep -Milliseconds 200
$wshell.SendKeys("^f")
Start-Sleep -Milliseconds 100
Set-Clipboard -Value %q
$wshell.SendKeys("^v")
Start-Sleep -Milliseconds 300
$wshell.SendKeys("{ENTER}")
Start-Sleep -Milliseconds 200
Set-Clipboard -Value %q
$wshell.SendKeys("^v")
Start-Sleep -Milliseconds 100
$wshell.SendKeys("{ENTER}")
`, recipient, content)

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("powershell send failed: %w, output: %s", err, string(out))
	}
	return nil
}
