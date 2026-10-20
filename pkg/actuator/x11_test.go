package actuator

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordedCommand records a command execution in mock runner.
type recordedCommand struct {
	Name  string
	Args  []string
	Stdin []byte
}

type mockCommandRunner struct {
	mu       sync.Mutex
	commands []recordedCommand
	handlers map[string]func(stdin []byte, args ...string) ([]byte, error)
	inFlight int32
	maxInFlight int32
}

func newMockCommandRunner() *mockCommandRunner {
	return &mockCommandRunner{
		handlers: make(map[string]func(stdin []byte, args ...string) ([]byte, error)),
	}
}

func (m *mockCommandRunner) Run(ctx context.Context, stdin []byte, name string, args ...string) ([]byte, error) {
	curr := atomic.AddInt32(&m.inFlight, 1)
	defer atomic.AddInt32(&m.inFlight, -1)

	m.mu.Lock()
	if curr > m.maxInFlight {
		m.maxInFlight = curr
	}
	m.commands = append(m.commands, recordedCommand{
		Name:  name,
		Args:  args,
		Stdin: stdin,
	})
	m.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	key := name + " " + strings.Join(args, " ")
	m.mu.Lock()
	handler, ok := m.handlers[key]
	if !ok {
		// prefix match or command name match
		handler = m.handlers[name]
	}
	m.mu.Unlock()

	if handler != nil {
		return handler(stdin, args...)
	}
	return []byte(""), nil
}

func (m *mockCommandRunner) getCommands() []recordedCommand {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]recordedCommand, len(m.commands))
	copy(res, m.commands)
	return res
}

func TestValidation(t *testing.T) {
	act := NewX11Actuator()
	act.DryRun = true
	ctx := context.Background()

	tests := []struct {
		name      string
		recipient string
		content   string
		wantErr   error
	}{
		{
			name:      "empty recipient",
			recipient: "",
			content:   "hello",
			wantErr:   ErrEmptyRecipient,
		},
		{
			name:      "whitespace recipient",
			recipient: "   ",
			content:   "hello",
			wantErr:   ErrEmptyRecipient,
		},
		{
			name:      "empty content",
			recipient: "filehelper",
			content:   "",
			wantErr:   ErrEmptyContent,
		},
		{
			name:      "whitespace content",
			recipient: "filehelper",
			content:   "   ",
			wantErr:   ErrEmptyContent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := act.SendMessage(ctx, tc.recipient, tc.content)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("expected error %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestDryRun(t *testing.T) {
	mock := newMockCommandRunner()
	act := NewX11Actuator()
	act.DryRun = true
	act.runner = mock

	err := act.SendMessage(context.Background(), "filehelper", "hello test")
	if err != nil {
		t.Fatalf("unexpected error on DryRun: %v", err)
	}

	if len(mock.getCommands()) != 0 {
		t.Errorf("expected 0 commands executed in DryRun, got %d", len(mock.getCommands()))
	}
}

func TestContextCancelled(t *testing.T) {
	act := NewX11Actuator()
	act.DryRun = true

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := act.SendMessage(ctx, "filehelper", "hello")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestWeChatNotFound(t *testing.T) {
	mock := newMockCommandRunner()
	// getactivewindow returns 1001
	mock.handlers["xdotool getactivewindow"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("1001\n"), nil
	}
	// xclip -selection clipboard -o returns initial clipboard
	mock.handlers["xclip -selection clipboard -o"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("prior-clipboard"), nil
	}
	// xdotool search returns empty
	mock.handlers["xdotool search --onlyvisible --class wechat"] = func(stdin []byte, args ...string) ([]byte, error) {
		return nil, errors.New("exit code 1")
	}

	act := NewX11Actuator()
	act.runner = mock
	act.searchDelay = 1 * time.Millisecond
	act.sendDelay = 1 * time.Millisecond

	err := act.SendMessage(context.Background(), "filehelper", "test msg")
	if err == nil {
		t.Fatalf("expected error when wechat window not found, got nil")
	}
	if !errors.Is(err, ErrWeChatNotFound) {
		t.Errorf("expected ErrWeChatNotFound, got %v", err)
	}

	// Verify clipboard and window were restored even when wechat was not found!
	cmds := mock.getCommands()
	var restoredClipboard bool
	var restoredWindow bool
	for _, cmd := range cmds {
		if cmd.Name == "xclip" && bytes.Equal(cmd.Stdin, []byte("prior-clipboard")) {
			restoredClipboard = true
		}
		if cmd.Name == "xdotool" && len(cmd.Args) >= 2 && cmd.Args[0] == "windowactivate" && cmd.Args[len(cmd.Args)-1] == "1001" {
			restoredWindow = true
		}
	}
	if !restoredClipboard {
		t.Errorf("expected original clipboard to be restored on error")
	}
	if !restoredWindow {
		t.Errorf("expected original window to be restored on error")
	}
}

func TestSendSuccessFlow(t *testing.T) {
	mock := newMockCommandRunner()
	mock.handlers["xdotool getactivewindow"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("1001\n"), nil
	}
	mock.handlers["xclip -selection clipboard -o"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("saved-clipboard-data"), nil
	}
	mock.handlers["xdotool search --onlyvisible --class wechat"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("2002\n"), nil
	}

	act := NewX11Actuator()
	act.runner = mock
	act.searchDelay = 1 * time.Millisecond
	act.sendDelay = 1 * time.Millisecond

	recipient := "文件传输助手"
	content := "你好，这是自动化测试消息！"

	err := act.SendMessage(context.Background(), recipient, content)
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	cmds := mock.getCommands()

	// Verify CJK is passed via xclip Stdin, NEVER via xdotool type
	for _, cmd := range cmds {
		if cmd.Name == "xdotool" {
			for _, arg := range cmd.Args {
				if arg == "type" {
					t.Fatalf("xdotool type MUST NOT be used for CJK text: found command %v", cmd)
				}
			}
		}
	}

	// Check that recipient and content were written to clipboard
	var wroteRecipient, wroteContent, restoredClipboard, activatedWechat, restoredWindow bool
	for _, cmd := range cmds {
		if cmd.Name == "xclip" {
			if string(cmd.Stdin) == recipient {
				wroteRecipient = true
			}
			if string(cmd.Stdin) == content {
				wroteContent = true
			}
			if string(cmd.Stdin) == "saved-clipboard-data" {
				restoredClipboard = true
			}
		}
		if cmd.Name == "xdotool" {
			if len(cmd.Args) >= 2 && cmd.Args[0] == "windowactivate" {
				if cmd.Args[len(cmd.Args)-1] == "2002" {
					activatedWechat = true
				}
				if cmd.Args[len(cmd.Args)-1] == "1001" {
					restoredWindow = true
				}
			}
		}
	}

	if !wroteRecipient {
		t.Errorf("recipient was not written to clipboard")
	}
	if !wroteContent {
		t.Errorf("content was not written to clipboard")
	}
	if !restoredClipboard {
		t.Errorf("original clipboard was not restored")
	}
	if !activatedWechat {
		t.Errorf("wechat window was not activated")
	}
	if !restoredWindow {
		t.Errorf("original active window was not restored")
	}
}

func TestMultipleWindowsSelection(t *testing.T) {
	mock := newMockCommandRunner()
	mock.handlers["xdotool getactivewindow"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("1001\n"), nil
	}
	mock.handlers["xclip -selection clipboard -o"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte(""), nil
	}
	// returns 3 windows: 3001 (10x10), 3002 (880x755 - main), 3003 (700x640)
	mock.handlers["xdotool search --onlyvisible --class wechat"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("3001\n3002\n3003\n"), nil
	}
	mock.handlers["xdotool getwindowgeometry 3001"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("Window 3001\n  Geometry: 10x10\n"), nil
	}
	mock.handlers["xdotool getwindowgeometry 3002"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("Window 3002\n  Geometry: 880x755\n"), nil
	}
	mock.handlers["xdotool getwindowgeometry 3003"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("Window 3003\n  Geometry: 700x640\n"), nil
	}

	act := NewX11Actuator()
	act.runner = mock
	act.searchDelay = 1 * time.Millisecond
	act.sendDelay = 1 * time.Millisecond

	err := act.SendMessage(context.Background(), "filehelper", "test")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	cmds := mock.getCommands()
	var activatedMain bool
	for _, cmd := range cmds {
		if cmd.Name == "xdotool" && len(cmd.Args) >= 2 && cmd.Args[0] == "windowactivate" && cmd.Args[len(cmd.Args)-1] == "3002" {
			activatedMain = true
		}
	}
	if !activatedMain {
		t.Errorf("expected window 3002 (largest geometry 880x755) to be activated")
	}
}

func TestMutexConcurrency(t *testing.T) {
	mock := newMockCommandRunner()
	mock.handlers["xdotool getactivewindow"] = func(stdin []byte, args ...string) ([]byte, error) {
		time.Sleep(10 * time.Millisecond)
		return []byte("1001\n"), nil
	}
	mock.handlers["xclip -selection clipboard -o"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte(""), nil
	}
	mock.handlers["xdotool search --onlyvisible --class wechat"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("2002\n"), nil
	}

	act := NewX11Actuator()
	act.runner = mock
	act.searchDelay = 2 * time.Millisecond
	act.sendDelay = 2 * time.Millisecond

	var wg sync.WaitGroup
	workers := 5
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_ = act.SendMessage(context.Background(), "filehelper", "concurrent msg")
		}()
	}

	wg.Wait()

	// Since X11Actuator uses sync.Mutex around SendMessage,
	// maxInFlight in runner must never exceed 1!
		if mock.maxInFlight > 1 {
			t.Errorf("mutex failed to serialize calls: maxInFlight was %d, expected 1", mock.maxInFlight)
		}
}

func TestMatchRecipient(t *testing.T) {
	tests := []struct {
		header    string
		recipient string
		want      bool
	}{
		{"莫宏涛 14770697743", "莫宏涛", true},
		{"莫宏涛 14770697743", "14770697743", true},
		{"文件传输助手", "filehelper", true},
		{"filehelper", "文件传输助手", true},
		{"莫宏涛 14770697743", "文件传输助手", false},
		{"", "文件传输助手", false},
		{"文件传输助手", "", false},
	}

	for _, tc := range tests {
		got := MatchRecipient(tc.header, tc.recipient)
		if got != tc.want {
			t.Errorf("MatchRecipient(%q, %q) = %v, want %v", tc.header, tc.recipient, got, tc.want)
		}
	}
}

func TestSafetyAbortOnMismatch(t *testing.T) {
	mock := newMockCommandRunner()
	mock.handlers["xdotool getactivewindow"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("1001\n"), nil
	}
	mock.handlers["xclip -selection clipboard -o"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("orig-clip"), nil
	}
	mock.handlers["xdotool search --onlyvisible --class wechat"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("2002\n"), nil
	}
	mock.handlers["import"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte(""), nil
	}
	// OCR reports current active chat is "莫宏涛 14770697743"
	mock.handlers["tesseract"] = func(stdin []byte, args ...string) ([]byte, error) {
		return []byte("莫宏涛 14770697743\n"), nil
	}

	act := NewX11Actuator()
	act.runner = mock
	act.searchDelay = 1 * time.Millisecond
	act.sendDelay = 1 * time.Millisecond

	// Attempt to send to "文件传输助手", but active chat is "莫宏涛"
	err := act.SendMessage(context.Background(), "文件传输助手", "绝不能误发的机密内容")
	if err == nil {
		t.Fatalf("expected error on recipient mismatch, got nil")
	}
	if !errors.Is(err, ErrRecipientMismatch) {
		t.Errorf("expected ErrRecipientMismatch, got %v", err)
	}

	// Verify that the sensitive content was NEVER written to clipboard!
	cmds := mock.getCommands()
	for _, cmd := range cmds {
		if cmd.Name == "xclip" && string(cmd.Stdin) == "绝不能误发的机密内容" {
			t.Fatalf("CRITICAL SAFETY FAILURE: message content was written to clipboard despite mismatch!")
		}
	}
}
