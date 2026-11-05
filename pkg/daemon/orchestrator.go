package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/inailong/wechat_mcp/pkg/brain"
	"github.com/inailong/wechat_mcp/pkg/bridge"
	"github.com/inailong/wechat_mcp/pkg/daemon/config"
	"github.com/inailong/wechat_mcp/pkg/prompt"
)

type DashboardLogger interface {
	AddLog(sender, incoming, reply, latency string)
}

type ReplyAuditRecord struct {
	Timestamp string `json:"timestamp"`
	SessionID string `json:"sessionId"`
	Sender    string `json:"sender"`
	Incoming  string `json:"incoming"`
	Reply     string `json:"reply"`
	LatencyMs int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

type DaemonOption func(*Daemon)

func WithMessageSource(source MessageSource) DaemonOption {
	return func(d *Daemon) {
		d.source = source
	}
}

func WithLogPath(path string) DaemonOption {
	return func(d *Daemon) {
		d.logPath = path
	}
}

func WithDashboard(dash DashboardLogger) DaemonOption {
	return func(d *Daemon) {
		d.dashboard = dash
	}
}

type senderDebouncer struct {
	mu       sync.Mutex
	timer    *time.Timer
	messages []InboundMessage
}

type Daemon struct {
	cfg         *config.DaemonConfig
	promptStore *prompt.Store
	brain       *brain.Client
	bClient     *bridge.Client
	source      MessageSource
	dashboard   DashboardLogger
	logPath     string

	debouncers   map[string]*senderDebouncer
	debouncersMu sync.Mutex
}

func NewDaemon(cfg *config.DaemonConfig, pStore *prompt.Store, bClient *brain.Client, bridgeClient *bridge.Client, opts ...DaemonOption) *Daemon {
	defaultLogPath := "/home/sesley/gdrive/00_workspace/zcode/wechat_replies.jsonl"
	d := &Daemon{
		cfg:         cfg,
		promptStore: pStore,
		brain:       bClient,
		bClient:     bridgeClient,
		logPath:     defaultLogPath,
		debouncers:  make(map[string]*senderDebouncer),
	}

	for _, opt := range opts {
		opt(d)
	}

	if d.source == nil {
		d.source = NewWCDBWatcher(bridgeClient)
	}

	return d
}

func (d *Daemon) Start(ctx context.Context) error {
	// If source is runnable, run it in background
	if watcher, ok := d.source.(*WCDBWatcher); ok {
		go func() {
			_ = watcher.Start(ctx)
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-d.source.Messages():
			if !ok {
				return nil
			}
			d.handleInbound(ctx, msg)
		}
	}
}

func (d *Daemon) handleInbound(ctx context.Context, msg InboundMessage) {
	// 1. Whitelist gate: check if sender or session is enabled
	if d.cfg == nil {
		return
	}
	targetID := msg.SessionID
	if targetID == "" {
		targetID = msg.Sender
	}

	if !d.cfg.IsEnabled(targetID) && !d.cfg.IsEnabled(msg.Sender) {
		return // Ignore disabled contacts
	}

	fmt.Printf("[daemon] Inbound message from enabled contact %s (%s): %q\n", msg.Sender, targetID, msg.Content)

	// 2. Debouncer
	d.debouncersMu.Lock()
	deb, ok := d.debouncers[targetID]
	if !ok {
		deb = &senderDebouncer{}
		d.debouncers[targetID] = deb
	}
	d.debouncersMu.Unlock()

	deb.mu.Lock()
	deb.messages = append(deb.messages, msg)
	if deb.timer != nil {
		deb.timer.Stop()
	}

	debounceSec := 3
	if d.cfg != nil && d.cfg.DebounceSec > 0 {
		debounceSec = d.cfg.DebounceSec
	}

	deb.timer = time.AfterFunc(time.Duration(debounceSec)*time.Second, func() {
		d.processDebouncedMessages(ctx, targetID, deb)
	})
	deb.mu.Unlock()
}

func (d *Daemon) processDebouncedMessages(ctx context.Context, targetID string, deb *senderDebouncer) {
	deb.mu.Lock()
	msgs := make([]InboundMessage, len(deb.messages))
	copy(msgs, deb.messages)
	deb.messages = nil
	deb.timer = nil
	deb.mu.Unlock()

	if len(msgs) == 0 {
		return
	}

	start := time.Now()

	// 1. Combine content and collect images
	var contents []string
	var images []string
	for _, m := range msgs {
		if m.Content != "" {
			contents = append(contents, m.Content)
		}
		images = append(images, m.ImagePaths...)
	}
	combinedText := strings.Join(contents, "\n")

	// 2. Fetch custom prompt
	systemPrompt := ""
	if d.promptStore != nil {
		promptText, err := d.promptStore.GetPrompt(targetID)
		if err == nil {
			systemPrompt = promptText
		}
	}

	// 3. Assemble history
	history := []brain.ChatMessage{
		{
			Role:       "user",
			Content:    combinedText,
			ImagePaths: images,
		},
	}

	// 4. Invoke Brain
	var reply string
	var err error
	if d.brain != nil {
		reply, err = d.brain.GenerateReply(ctx, systemPrompt, history)
	}

		var sendErr error
		if err == nil && reply != "" && d.bClient != nil {
			// 5. Send message via Bridge / X11 Actuator
			recipient := targetID
			if contact, cErr := d.bClient.GetContact(ctx, targetID); cErr == nil && contact != nil {
				if contact.DisplayName != "" {
					recipient = contact.DisplayName
				} else if contact.Remark != "" {
					recipient = contact.Remark
				} else if contact.Nickname != "" {
					recipient = contact.Nickname
				}
			}

			fmt.Printf("[daemon] Delivering auto-reply to %s (%s): %q\n", targetID, recipient, reply)
			sendErr = d.bClient.SendMessage(ctx, recipient, reply)
			if sendErr != nil {
				fmt.Printf("[daemon] SendMessage error to %s: %v\n", recipient, sendErr)
			} else {
				fmt.Printf("[daemon] Auto-reply delivered to %s in %v\n", recipient, time.Since(start))
			}
		}

	latency := time.Since(start)

	// 6. Record audit log
	errStr := ""
	if err != nil {
		errStr = "brain error: " + err.Error()
	} else if sendErr != nil {
		errStr = "send error: " + sendErr.Error()
	}

	record := ReplyAuditRecord{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		SessionID: targetID,
		Sender:    msgs[0].Sender,
		Incoming:  combinedText,
		Reply:     reply,
		LatencyMs: latency.Milliseconds(),
		Error:     errStr,
	}

	d.writeLog(record)

	if d.dashboard != nil {
		d.dashboard.AddLog(targetID, combinedText, reply, fmt.Sprintf("%dms", latency.Milliseconds()))
	}
}

func (d *Daemon) writeLog(record ReplyAuditRecord) {
	if d.logPath == "" {
		return
	}

	_ = os.MkdirAll(filepath.Dir(d.logPath), 0755)
	line, err := json.Marshal(record)
	if err != nil {
		return
	}

	f, err := os.OpenFile(d.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.Write(append(line, '\n'))
}
