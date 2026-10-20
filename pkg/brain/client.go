package brain

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ChatMessage struct {
	Role       string   `json:"role"`
	Content    string   `json:"content"`
	ImagePaths []string `json:"imagePaths,omitempty"`
}

type Client struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey, model string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	if model == "" {
		model = "gemini-3.8-flash-high"
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type openAIChatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type openAIChatRequest struct {
	Model     string              `json:"model"`
	Messages  []openAIChatMessage `json:"messages"`
	MaxTokens int                 `json:"max_tokens,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *Client) GenerateReply(ctx context.Context, systemPrompt string, history []ChatMessage) (string, error) {
	var messages []openAIChatMessage

	// 1. System prompt
	if systemPrompt != "" {
		messages = append(messages, openAIChatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// 2. Chat history
	for _, msg := range history {
		if len(msg.ImagePaths) == 0 {
			messages = append(messages, openAIChatMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
			continue
		}

		// Multimodal content array
		var contentParts []map[string]interface{}
		if msg.Content != "" {
			contentParts = append(contentParts, map[string]interface{}{
				"type": "text",
				"text": msg.Content,
			})
		}

		for _, imgPath := range msg.ImagePaths {
			imgData, err := os.ReadFile(imgPath)
			if err != nil {
				continue
			}
			mimeType := getMimeType(imgPath)
			b64 := base64.StdEncoding.EncodeToString(imgData)
			dataURI := fmt.Sprintf("data:%s;base64,%s", mimeType, b64)

			contentParts = append(contentParts, map[string]interface{}{
				"type": "image_url",
				"image_url": map[string]string{
					"url": dataURI,
				},
			})
		}

		messages = append(messages, openAIChatMessage{
			Role:    msg.Role,
			Content: contentParts,
		})
	}

	reqBody := openAIChatRequest{
		Model:     c.model,
		Messages:  messages,
		MaxTokens: 8192, // Generous token allowance for rich reasoning and reply
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode request: %w", err)
	}

	reqURL := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("api request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty response choices from api")
	}

	reply := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	// Safety sanitize: strip forbidden scroll emoji if any slipped in
	reply = strings.ReplaceAll(reply, "📜", "")
	return reply, nil
}

func getMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
