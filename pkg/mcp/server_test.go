package mcp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/nowa277/wechat_mcp/pkg/actuator"
	"github.com/nowa277/wechat_mcp/pkg/bridge"
)

func newTestServer(dryRun bool) *Server {
	act := actuator.NewX11Actuator()
	act.DryRun = dryRun

	client, err := bridge.NewClient(bridge.ClientOptions{DryRun: dryRun})
	if err != nil {
		client = &bridge.Client{}
	}
	client.SetActuator(act)
	return NewServer(client)
}

func TestGetTools_SendMessage(t *testing.T) {
	s := newTestServer(true)
	tools := s.getTools()

	var sendMsgTool *Tool
	for i := range tools {
		if tools[i].Name == "send_message" {
			sendMsgTool = &tools[i]
			break
		}
	}

	if sendMsgTool == nil {
		t.Fatal("expected send_message tool to be registered in getTools()")
	}

	expectedDesc := "Send a message to a WeChat contact or chat via desktop client automation"
	if sendMsgTool.Description != expectedDesc {
		t.Errorf("expected description %q, got %q", expectedDesc, sendMsgTool.Description)
	}

	// Annotations should NOT have readOnlyHint: true
	if sendMsgTool.Annotations != nil {
		if m, ok := sendMsgTool.Annotations.(map[string]bool); ok && m["readOnlyHint"] {
			t.Errorf("send_message must NOT have readOnlyHint: true")
		}
	}

	// Verify other tools still have readOnlyHint: true
	for _, tool := range tools {
		if tool.Name == "list_sessions" || tool.Name == "list_messages" || tool.Name == "search_messages" || tool.Name == "get_contact" || tool.Name == "list_files" || tool.Name == "get_message_file" {
			m, ok := tool.Annotations.(map[string]bool)
			if !ok || !m["readOnlyHint"] {
				t.Errorf("tool %s should have readOnlyHint: true", tool.Name)
			}
		}
	}

	// Verify InputSchema
	schemaMap, ok := sendMsgTool.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatalf("expected InputSchema to be map[string]interface{}, got %T", sendMsgTool.InputSchema)
	}
	if schemaMap["type"] != "object" {
		t.Errorf("expected schema type 'object', got %v", schemaMap["type"])
	}

	props, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected properties in schema, got %T", schemaMap["properties"])
	}

	recProp, ok := props["recipient"].(map[string]interface{})
	if !ok || recProp["type"] != "string" {
		t.Errorf("expected recipient property of type string, got %v", props["recipient"])
	}

	contentProp, ok := props["content"].(map[string]interface{})
	if !ok || contentProp["type"] != "string" {
		t.Errorf("expected content property of type string, got %v", props["content"])
	}

	reqs, ok := schemaMap["required"].([]string)
	if !ok {
		t.Fatalf("expected required to be []string, got %T", schemaMap["required"])
	}
	reqMap := make(map[string]bool)
	for _, r := range reqs {
		reqMap[r] = true
	}
	if !reqMap["recipient"] || !reqMap["content"] {
		t.Errorf("expected recipient and content to be required, got %v", reqs)
	}
}

func TestHandleToolCall_SendMessage_Success(t *testing.T) {
	s := newTestServer(true)
	ctx := context.Background()

	res := s.handleToolCall(ctx, "send_message", map[string]interface{}{
		"recipient": "filehelper",
		"content":   "hello from test",
	})

	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res)
	}
	if len(res.Content) == 0 {
		t.Fatal("expected content in result")
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(res.Content[0].Text), &data); err != nil {
		t.Fatalf("failed to unmarshal result JSON: %v, raw: %s", err, res.Content[0].Text)
	}

	if data["success"] != true {
		t.Errorf("expected success: true, got %v", data["success"])
	}
	if data["recipient"] != "filehelper" {
		t.Errorf("expected recipient 'filehelper', got %v", data["recipient"])
	}
	if data["message"] != "sent successfully" {
		t.Errorf("expected message 'sent successfully', got %v", data["message"])
	}
}

func TestHandleToolCall_SendMessage_Validation(t *testing.T) {
	s := newTestServer(true)
	ctx := context.Background()

	tests := []struct {
		name      string
		args      map[string]interface{}
		errSubstr string
	}{
		{
			name:      "missing recipient",
			args:      map[string]interface{}{"content": "hello"},
			errSubstr: "recipient",
		},
		{
			name:      "empty recipient",
			args:      map[string]interface{}{"recipient": "", "content": "hello"},
			errSubstr: "recipient",
		},
		{
			name:      "whitespace recipient",
			args:      map[string]interface{}{"recipient": "   \t", "content": "hello"},
			errSubstr: "recipient",
		},
		{
			name:      "missing content",
			args:      map[string]interface{}{"recipient": "filehelper"},
			errSubstr: "content",
		},
		{
			name:      "empty content",
			args:      map[string]interface{}{"recipient": "filehelper", "content": ""},
			errSubstr: "content",
		},
		{
			name:      "whitespace content",
			args:      map[string]interface{}{"recipient": "filehelper", "content": "   \n  "},
			errSubstr: "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := s.handleToolCall(ctx, "send_message", tt.args)
			if !res.IsError {
				t.Fatalf("expected error result, got success: %+v", res)
			}
			if len(res.Content) == 0 {
				t.Fatal("expected error content")
			}
			if !strings.Contains(res.Content[0].Text, tt.errSubstr) {
				t.Errorf("expected error containing %q, got %q", tt.errSubstr, res.Content[0].Text)
			}
		})
	}
}

func TestHandleToolCall_SendMessage_ContextCancelled(t *testing.T) {
	s := newTestServer(false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := s.handleToolCall(ctx, "send_message", map[string]interface{}{
		"recipient": "filehelper",
		"content":   "hello",
	})

	if !res.IsError {
		t.Fatalf("expected error result for cancelled context, got success: %+v", res)
	}
	if len(res.Content) == 0 {
		t.Fatal("expected error content")
	}
	if !strings.Contains(res.Content[0].Text, "context canceled") {
		t.Errorf("expected 'context canceled' in error, got: %s", res.Content[0].Text)
	}
}

func captureStdout(f func()) string {
	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)
	return string(out)
}

func TestHandleRequest_ToolsList(t *testing.T) {
	s := newTestServer(true)
	ctx := context.Background()

	out := captureStdout(func() {
		s.handleRequest(ctx, Request{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/list",
		})
	})

	var resp Response
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v, raw: %s", err, out)
	}

	resMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}
	tools, ok := resMap["tools"].([]interface{})
	if !ok {
		t.Fatalf("expected tools list, got %T", resMap["tools"])
	}

	var foundSendMessage bool
	for _, rawTool := range tools {
		toolMap, ok := rawTool.(map[string]interface{})
		if ok && toolMap["name"] == "send_message" {
			foundSendMessage = true
			break
		}
	}

	if !foundSendMessage {
		t.Error("expected tools/list to contain send_message")
	}
}

func TestHandleRequest_ToolsCall_SendMessage(t *testing.T) {
	s := newTestServer(true)
	ctx := context.Background()

	params, _ := json.Marshal(map[string]interface{}{
		"name": "send_message",
		"arguments": map[string]interface{}{
			"recipient": "filehelper",
			"content":   "test via jsonrpc",
		},
	})

	out := captureStdout(func() {
		s.handleRequest(ctx, Request{
			JSONRPC: "2.0",
			ID:      2,
			Method:  "tools/call",
			Params:  params,
		})
	})

	var resp Response
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v, raw: %s", err, out)
	}

	if resp.Error != nil {
		t.Fatalf("expected no JSON-RPC error, got %+v", resp.Error)
	}

	resMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}

	isErr, _ := resMap["isError"].(bool)
	if isErr {
		t.Fatalf("expected isError=false, got %+v", resMap)
	}

	contents, ok := resMap["content"].([]interface{})
	if !ok || len(contents) == 0 {
		t.Fatalf("expected non-empty content list, got %v", resMap["content"])
	}
	firstContent := contents[0].(map[string]interface{})
	text := firstContent["text"].(string)

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("failed to unmarshal tool result payload: %v", err)
	}

	if payload["success"] != true {
		t.Errorf("expected success: true, got %v", payload["success"])
	}
	if payload["recipient"] != "filehelper" {
		t.Errorf("expected recipient 'filehelper', got %v", payload["recipient"])
	}
}
