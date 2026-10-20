package bridge

import (
	"context"
	"errors"
	"testing"

	"github.com/nowa277/wechat_mcp/pkg/actuator"
)

func TestBridgeLive(t *testing.T) {
	client, err := NewClient(ClientOptions{
		BaseURL: "http://127.0.0.1:5031",
		Token:   "wechat_mcp_local_token_8888",
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()
	sessions, err := client.ListSessions(ctx, "", 5, 0)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions.Sessions) == 0 {
		t.Fatalf("Expected sessions > 0")
	}

	t.Logf("Got %d sessions. First session: %+v", len(sessions.Sessions), sessions.Sessions[0])

	// Read messages of first session
	firstSessID := sessions.Sessions[0].SessionID
	msgs, err := client.ListMessages(ctx, firstSessID, 3, 0, "desc", 0, 0)
	if err != nil {
		t.Fatalf("ListMessages failed: %v", err)
	}
	t.Logf("Got %d messages from %s. First msg: %+v", len(msgs.Messages), firstSessID, msgs.Messages[0])

	// Get contact of first session
	contact, err := client.GetContact(ctx, firstSessID)
	if err != nil {
		t.Logf("GetContact info: %v", err)
	} else {
		t.Logf("Contact: %+v", contact)
	}
}

func TestClientSendMessage_Validation(t *testing.T) {
	client, err := NewClient(ClientOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()

	tests := []struct {
		name      string
		recipient string
		content   string
		wantErr   error
	}{
		{"empty recipient", "", "hello", ErrEmptyRecipient},
		{"whitespace recipient", "   \t", "hello", ErrEmptyRecipient},
		{"empty content", "filehelper", "", ErrEmptyContent},
		{"whitespace content", "filehelper", "  \n  ", ErrEmptyContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SendMessage(ctx, tt.recipient, tt.content)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestClientSendMessage_DryRun(t *testing.T) {
	client, err := NewClient(ClientOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.Actuator() == nil {
		t.Fatal("expected actuator to be initialized")
	}
	if !client.Actuator().DryRun {
		t.Fatal("expected actuator.DryRun to be true when ClientOptions.DryRun is true")
	}

	ctx := context.Background()
	err = client.SendMessage(ctx, "filehelper", "hello test")
	if err != nil {
		t.Fatalf("expected nil error on dry run, got: %v", err)
	}
}

func TestClientSendMessage_SetActuator(t *testing.T) {
	client := &Client{}
	customAct := actuator.NewX11Actuator()
	customAct.DryRun = true

	client.SetActuator(customAct)
	if client.Actuator() != customAct {
		t.Fatal("expected custom actuator to be set")
	}

	ctx := context.Background()
	if err := client.SendMessage(ctx, "filehelper", "test message"); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestClientSendMessage_NilActuatorAutoInit(t *testing.T) {
	client := &Client{}
	// actuator is nil initially
	if client.Actuator() != nil {
		t.Fatal("expected actuator to be nil initially")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so actual X11 commands won't run

	_ = client.SendMessage(ctx, "filehelper", "test")
	if client.Actuator() == nil {
		t.Fatal("expected actuator to be automatically initialized upon SendMessage call")
	}
}

func TestClientSendMessage_ContextCancelled(t *testing.T) {
	client, err := NewClient(ClientOptions{DryRun: false})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = client.SendMessage(ctx, "filehelper", "test message")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}


