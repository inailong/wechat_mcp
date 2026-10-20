package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptStore_DefaultInit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "prompt_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Verify default.md was created
	defPath := filepath.Join(tmpDir, "default.md")
	if _, err := os.Stat(defPath); os.IsNotExist(err) {
		t.Fatalf("expected default.md to be created automatically")
	}

	// Get prompt for unknown user should return default prompt
	content, err := store.GetPrompt("unknown_friend")
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}
	if !strings.Contains(content, "儒雅") {
		t.Errorf("expected default prompt to contain '儒雅', got: %s", content)
	}
}

func TestPromptStore_CustomUserPrompt(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "prompt_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	customContent := "此人乃是我的挚友圣杰，为人豪爽，多有切磋。"
	username := "wxid_shengjie"
	err = store.SavePrompt(username, customContent)
	if err != nil {
		t.Fatalf("SavePrompt failed: %v", err)
	}

	got, err := store.GetPrompt(username)
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}
	if got != customContent {
		t.Errorf("GetPrompt = %q, want %q", got, customContent)
	}
}
