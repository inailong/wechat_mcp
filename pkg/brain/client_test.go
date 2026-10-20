package brain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrainClient_GenerateReply(t *testing.T) {
	var capturedReq map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/chat/completions" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		_ = json.NewDecoder(r.Body).Decode(&capturedReq)

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "兄台安好，方才瞧过此图，甚是精巧。",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "gemini-3.8-flash-high")

	// Create dummy image file
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "sample.jpg")
	_ = os.WriteFile(imgPath, []byte("fake-image-bytes"), 0644)

	systemPrompt := "你是儒雅助攻知己。"
	history := []ChatMessage{
		{
			Role:       "user",
			Content:    "请瞧瞧这图何如？",
			ImagePaths: []string{imgPath},
		},
	}

	reply, err := client.GenerateReply(context.Background(), systemPrompt, history)
	if err != nil {
		t.Fatalf("GenerateReply failed: %v", err)
	}

	if !strings.Contains(reply, "方才瞧过此图") {
		t.Errorf("unexpected reply: %s", reply)
	}

	// Verify request body
	if capturedReq["model"] != "gemini-3.8-flash-high" {
		t.Errorf("expected model gemini-3.8-flash-high, got %v", capturedReq["model"])
	}

	// Verify max_tokens is generous (at least 4096 or omitted)
	if maxTokens, ok := capturedReq["max_tokens"].(float64); ok && maxTokens < 4096 {
		t.Errorf("max_tokens was restricted to %v, must be at least 4096", maxTokens)
	}

	// Verify messages structure
	msgs, ok := capturedReq["messages"].([]interface{})
	if !ok || len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages (system + user), got %v", msgs)
	}
}
