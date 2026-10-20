package actuator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrEmptyRecipient indicates that the recipient is empty or contains only whitespace.
	ErrEmptyRecipient = errors.New("recipient cannot be empty")
	// ErrEmptyContent indicates that the message content is empty or contains only whitespace.
	ErrEmptyContent = errors.New("content cannot be empty")
	// ErrWeChatNotFound indicates that no visible WeChat window was found.
	ErrWeChatNotFound = errors.New("wechat window not found")
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

	// 5. Trigger search (Ctrl+F)
	if _, err := runner.Run(ctx, nil, "xdotool", "key", "--clearmodifiers", "ctrl+f"); err != nil {
		return fmt.Errorf("failed to press ctrl+f: %w", err)
	}

	if err := sleepWithContext(ctx, 50*time.Millisecond); err != nil {
		return err
	}

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

	if err := sleepWithContext(ctx, 100*time.Millisecond); err != nil {
		return err
	}

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
