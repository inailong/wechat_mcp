package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nowa277/wechat_mcp/pkg/daemon/config"
	"github.com/nowa277/wechat_mcp/pkg/prompt"
)

func TestDashboardServer_API(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	pStore, err := prompt.NewStore(filepath.Join(tmpDir, "prompts"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	server := NewServer(cfg, pStore, nil, 0)
	handler := server.Handler()

	// 1. Test GET / (embedded HTML UI)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for /, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "微信智能自动代答中枢") {
		t.Errorf("expected HTML content to contain brand title")
	}

	// 2. Test POST /api/toggle
	toggleBody := []byte(`{"username":"wxid_testuser","enabled":true}`)
	req = httptest.NewRequest("POST", "/api/toggle", bytes.NewReader(toggleBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for /api/toggle, got %d", rec.Code)
	}
	if !cfg.IsEnabled("wxid_testuser") {
		t.Errorf("expected wxid_testuser to be enabled in config")
	}

	// 3. Test GET & POST /api/prompt
	customPrompt := "专属人设：此乃同窗挚友。"
	saveBody, _ := json.Marshal(map[string]string{
		"username": "wxid_testuser",
		"prompt":   customPrompt,
	})
	req = httptest.NewRequest("POST", "/api/prompt", bytes.NewReader(saveBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for POST /api/prompt, got %d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/api/prompt?username=wxid_testuser", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var promptResp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &promptResp)
	if promptResp["prompt"] != customPrompt {
		t.Errorf("expected custom prompt %q, got %q", customPrompt, promptResp["prompt"])
	}
}
