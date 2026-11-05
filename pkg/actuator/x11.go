//go:build linux

package actuator

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var geomRegex = regexp.MustCompile(`Geometry:\s*(\d+)x(\d+)`)

// CommandRunner defines the interface for executing system commands.
type CommandRunner interface {
	Run(ctx context.Context, stdin []byte, name string, args ...string) ([]byte, error)
}

type defaultCommandRunner struct{}

func (r *defaultCommandRunner) Run(ctx context.Context, stdin []byte, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	// In X11, xclip write (without -o) daemonizes to serve the selection and child processes inherit stdout/stderr pipes.
	// If stdout/stderr are connected to Go pipe buffers, cmd.Run() hangs indefinitely waiting for EOF.
	// Using os.DevNull prevents Go from waiting on inherited pipes.
	if name == "xclip" {
		isOutput := false
		for _, arg := range args {
			if arg == "-o" {
				isOutput = true
				break
			}
		}
		if !isOutput {
			devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
			if err == nil {
				defer devNull.Close()
				cmd.Stdout = devNull
				cmd.Stderr = devNull
				return nil, cmd.Run()
			}
		}
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, errOut.String())
	}
	return out.Bytes(), nil
}

// X11Actuator automates message sending via X11 desktop automation (xdotool + xclip).
type X11Actuator struct {
	mu          sync.Mutex
	DryRun      bool
	runner      CommandRunner
	searchDelay time.Duration
	sendDelay   time.Duration
}

// NewX11Actuator creates a new X11 message actuator.
func NewX11Actuator() *X11Actuator {
	return &X11Actuator{
		runner:      &defaultCommandRunner{},
		searchDelay: 250 * time.Millisecond,
		sendDelay:   100 * time.Millisecond,
	}
}

func (a *X11Actuator) SetDryRun(dryRun bool) {
	a.DryRun = dryRun
}

func (a *X11Actuator) IsDryRun() bool {
	return a.DryRun
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func (a *X11Actuator) findWeChatWindow(ctx context.Context, runner CommandRunner) (string, error) {
	out, err := runner.Run(ctx, nil, "xdotool", "search", "--onlyvisible", "--class", "wechat")
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		out, err = runner.Run(ctx, nil, "xdotool", "search", "--class", "wechat")
	}
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return "", ErrWeChatNotFound
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var wids []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			wids = append(wids, l)
		}
	}
	if len(wids) == 0 {
		return "", ErrWeChatNotFound
	}

	if len(wids) == 1 {
		return wids[0], nil
	}

	bestWID := wids[0]
	maxArea := 0
	for _, wid := range wids {
		geomOut, err := runner.Run(ctx, nil, "xdotool", "getwindowgeometry", wid)
		if err != nil {
			continue
		}
		matches := geomRegex.FindSubmatch(geomOut)
		if len(matches) == 3 {
			w, _ := strconv.Atoi(string(matches[1]))
			h, _ := strconv.Atoi(string(matches[2]))
			area := w * h
			if area > maxArea {
				maxArea = area
				bestWID = wid
			}
		}
	}

	return bestWID, nil
}

// SendMessage sends a message to the specified recipient via WeChat X11 GUI automation.
// It is protected by a mutex to ensure sequential execution.
func (a *X11Actuator) SendMessage(ctx context.Context, recipient, content string) error {
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

	if a.DryRun {
		return nil
	}

	runner := a.runner
	if runner == nil {
		runner = &defaultCommandRunner{}
	}

	// 1. Capture current active window ID
	origWinBytes, _ := runner.Run(ctx, nil, "xdotool", "getactivewindow")
	origWinID := strings.TrimSpace(string(origWinBytes))

	// 2. Capture current clipboard content
	origClipBytes, _ := runner.Run(ctx, nil, "xclip", "-selection", "clipboard", "-o")

	// Ensure clipboard and active window are restored on exit
	defer func() {
		// Restore clipboard if previous content existed
		if len(origClipBytes) > 0 {
			_, _ = runner.Run(context.Background(), origClipBytes, "xclip", "-selection", "clipboard")
		}
		// Restore active window focus
		if origWinID != "" {
			_, _ = runner.Run(context.Background(), nil, "xdotool", "windowactivate", origWinID)
		}
	}()

	// 3. Find WeChat main window
	wechatWID, err := a.findWeChatWindow(ctx, runner)
	if err != nil {
		return err
	}

	// 4. Activate WeChat window
	_, err = runner.Run(ctx, nil, "xdotool", "windowactivate", "--sync", wechatWID)
	if err != nil {
		return fmt.Errorf("failed to activate wechat window: %w", err)
	}

	if err := sleepWithContext(ctx, 50*time.Millisecond); err != nil {
		return err
	}

		// 5. Focus search input (click search box at x=165, y=57 with fallback to Ctrl+F)
		if _, err := runner.Run(ctx, nil, "xdotool", "mousemove", "--window", wechatWID, "165", "57", "click", "1"); err != nil {
			_, _ = runner.Run(ctx, nil, "xdotool", "key", "--clearmodifiers", "ctrl+f")
		}

		if err := sleepWithContext(ctx, 100*time.Millisecond); err != nil {
			return err
		}

		// Clear search box
		_, _ = runner.Run(ctx, nil, "xdotool", "key", "--clearmodifiers", "ctrl+a", "BackSpace")
		_ = sleepWithContext(ctx, 50*time.Millisecond)

		// 6. Write recipient to clipboard and paste (Ctrl+V)
		if _, err := runner.Run(ctx, []byte(recipient), "xclip", "-selection", "clipboard"); err != nil {
			return fmt.Errorf("failed to copy recipient to clipboard: %w", err)
		}

		if _, err := runner.Run(ctx, nil, "xdotool", "key", "--clearmodifiers", "ctrl+v"); err != nil {
			return fmt.Errorf("failed to paste recipient: %w", err)
		}

		// 7. Wait search delay (e.g. 250ms) then press Return to enter chat
		if err := sleepWithContext(ctx, a.searchDelay); err != nil {
			return err
		}

		if _, err := runner.Run(ctx, nil, "xdotool", "key", "--clearmodifiers", "Return"); err != nil {
			return fmt.Errorf("failed to press return for search: %w", err)
		}

			// Safety check: poll active chat header until X11 repaint matches recipient (up to 1s)
			var activeTitle string
			deadline := time.Now().Add(1 * time.Second)
			for time.Now().Before(deadline) {
				t, err := a.GetActiveChatTitle(ctx, runner, wechatWID)
				if err == nil && strings.TrimSpace(t) != "" {
					activeTitle = t
					if MatchRecipient(t, recipient) {
						break
					}
				}
				_ = sleepWithContext(ctx, 100*time.Millisecond)
			}
			if strings.TrimSpace(activeTitle) != "" && !MatchRecipient(activeTitle, recipient) {
				return fmt.Errorf("%w: active chat header is %q, target recipient is %q", ErrRecipientMismatch, activeTitle, recipient)
			}

		// Ensure chat input box is focused (click at x=500, y=650)
		_, _ = runner.Run(ctx, nil, "xdotool", "mousemove", "--window", wechatWID, "500", "650", "click", "1")
		_ = sleepWithContext(ctx, 50*time.Millisecond)

		// 8. Write message content to clipboard and paste (Ctrl+V)
	if _, err := runner.Run(ctx, []byte(content), "xclip", "-selection", "clipboard"); err != nil {
		return fmt.Errorf("failed to copy content to clipboard: %w", err)
	}

	if _, err := runner.Run(ctx, nil, "xdotool", "key", "--clearmodifiers", "ctrl+v"); err != nil {
		return fmt.Errorf("failed to paste content: %w", err)
	}

	// 9. Wait send delay (e.g. 100ms) then press Return to send
	if err := sleepWithContext(ctx, a.sendDelay); err != nil {
		return err
	}

	if _, err := runner.Run(ctx, nil, "xdotool", "key", "--clearmodifiers", "Return"); err != nil {
		return fmt.Errorf("failed to press return to send: %w", err)
	}

	if err := sleepWithContext(ctx, 50*time.Millisecond); err != nil {
		return err
	}

	return nil
}

// GetActiveChatTitle uses X11 window screenshot and OCR to read the active conversation header.
func (a *X11Actuator) GetActiveChatTitle(ctx context.Context, runner CommandRunner, wid string) (string, error) {
	tmpImg := filepath.Join(os.TempDir(), fmt.Sprintf("wechat_hdr_%d.png", time.Now().UnixNano()))
	defer os.Remove(tmpImg)

	_, err := runner.Run(ctx, nil, "import", "-window", wid, "-crop", "450x60+310+35", tmpImg)
	if err != nil {
		return "", err
	}

	out, err := runner.Run(ctx, nil, "tesseract", tmpImg, "stdout", "-l", "chi_sim+eng", "--psm", "6")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

// MatchRecipient compares the detected header text with the target recipient identifier.
func MatchRecipient(headerTitle, recipient string) bool {
	h := strings.ToLower(strings.ReplaceAll(headerTitle, " ", ""))
	r := strings.ToLower(strings.ReplaceAll(recipient, " ", ""))
	if h == "" || r == "" {
		return false
	}
	if (r == "filehelper" && strings.Contains(h, "文件传输助手")) ||
		(strings.Contains(r, "文件传输助手") && strings.Contains(h, "filehelper")) {
		return true
	}
	if strings.Contains(h, r) || strings.Contains(r, h) {
		return true
	}
	// Check word tokens (e.g. "Chang", "Shengjie")
	tokens := strings.Fields(strings.ToLower(recipient))
	for _, tok := range tokens {
		if len(tok) >= 3 && strings.Contains(h, tok) {
			return true
		}
	}
	return false
}
