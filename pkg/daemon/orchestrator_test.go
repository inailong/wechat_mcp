package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nowa277/wechat_mcp/pkg/brain"
	"github.com/nowa277/wechat_mcp/pkg/bridge"
	"github.com/nowa277/wechat_mcp/pkg/daemon/config"
	"github.com/nowa277/wechat_mcp/pkg/prompt"
)

type mockMessageSource struct {
	msgChan chan InboundMessage
}

func (m *mockMessageSource) Messages() <-chan InboundMessage {
	return m.msgChan
}

func TestDaemon_DebounceAndWhitelist(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "daemon_config.json")
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	cfg.DebounceSec = 1 // 1s for fast test

	// Enable user A, user B is disabled
	userA := "wxid_user_a"
	userB := "wxid_user_b"
	_ = cfg.SetEnabled(userA, true)
	_ = cfg.SetEnabled(userB, false)

	pStore, _ := prompt.NewStore(filepath.Join(tmpDir, "prompts"))

	var brainCalls int32
	var brainPromptCaptured string
	var brainHistoryCaptured []brain.ChatMessage
	var brainMu sync.Mutex

	mockBrainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&brainCalls, 1)

		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		brainMu.Lock()
		if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
			brainPromptCaptured = req.Messages[0].Content
		}
		brainHistoryCaptured = nil
		for _, m := range req.Messages {
			if m.Role != "system" {
				brainHistoryCaptured = append(brainHistoryCaptured, brain.ChatMessage{
					Role:    m.Role,
					Content: m.Content,
				})
			}
		}
		brainMu.Unlock()

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "得空闲叙，安好无恙。",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockBrainServer.Close()

	brainClient := brain.NewClient(mockBrainServer.URL, "test-key", "gemini-3.8-flash-high")
	bClient, _ := bridge.NewClient(bridge.ClientOptions{DryRun: true})

	mockSource := &mockMessageSource{msgChan: make(chan InboundMessage, 10)}
	logFile := filepath.Join(tmpDir, "replies.jsonl")

	d := NewDaemon(cfg, pStore, brainClient, bClient, WithMessageSource(mockSource), WithLogPath(logFile))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = d.Start(ctx)
	}()

	// 1. Send message from disabled user B
	mockSource.msgChan <- InboundMessage{
		Sender:    userB,
		SessionID: userB,
		Content:   "你好在吗",
		Timestamp: time.Now().Unix(),
	}

	// Wait 1.5s (longer than debounce 1s)
	time.Sleep(1500 * time.Millisecond)

	if atomic.LoadInt32(&brainCalls) != 0 {
		t.Fatalf("expected 0 calls for disabled user B, got %d", brainCalls)
	}

	// 2. Send 2 rapid messages from enabled user A within 200ms
	now := time.Now().Unix()
	mockSource.msgChan <- InboundMessage{
		Sender:    userA,
		SessionID: userA,
		Content:   "兄长",
		Timestamp: now,
	}
	time.Sleep(200 * time.Millisecond)
	mockSource.msgChan <- InboundMessage{
		Sender:    userA,
		SessionID: userA,
		Content:   "今日可有空叙旧？",
		Timestamp: now + 1,
	}

	// Wait debounce window to complete (1s + margin)
	time.Sleep(1500 * time.Millisecond)

	// Verify debounce: only 1 brain call made for the 2 rapid messages!
	if calls := atomic.LoadInt32(&brainCalls); calls != 1 {
		t.Fatalf("expected exactly 1 brain call after debounce, got %d", calls)
	}

	// Verify JSONL log was written
	logData, err := os.ReadFile(logFile)
	if err != nil || len(logData) == 0 {
		t.Fatalf("expected jsonl log file to be written, err: %v", err)
	}

	brainMu.Lock()
	defer brainMu.Unlock()
	if brainPromptCaptured == "" {
		t.Errorf("expected system prompt to be passed to brain")
	}
	if len(brainHistoryCaptured) == 0 {
		t.Errorf("expected history to be passed to brain")
	}
}
