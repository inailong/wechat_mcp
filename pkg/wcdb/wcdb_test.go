package wcdb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeWCDBDirect(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	sessionDb := filepath.Join(home, "xwechat_files", "wxid_n9x0u8sjzc7f22_c6a1", "db_storage", "session", "session.db")
	realKey := "973ef9cbbc344455a0bd9d8cd8e97721fd1297c6f5dd43bfa67afeb2f3e45bf4"

	if _, err := os.Stat(sessionDb); os.IsNotExist(err) {
		t.Skip("session.db not found on this machine")
	}

	handle, err := OpenAccount(sessionDb, realKey)
	if err != nil {
		t.Fatalf("OpenAccount failed: %v", err)
	}
	t.Logf("Successfully opened account! Handle: %d", handle)

	sessions, err := GetSessions()
	if err != nil {
		t.Fatalf("GetSessions failed: %v", err)
	}
	t.Logf("Got %d sessions directly via CGO!", len(sessions))
	if len(sessions) == 0 {
		t.Fatalf("Expected sessions > 0")
	}

	first := sessions[0]
	t.Logf("First session: %v (%v)", first["username"], first["displayName"])

	// Test GetMessages on first group
	msgs, err := GetMessages("44858144348@chatroom", 3, 0)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	t.Logf("Got %d messages directly from message_0.db!", len(msgs))
	if len(msgs) > 0 {
		t.Logf("Latest msg: %v", msgs[0]["message_content"])
	}

	// Test GetContact
	contact, err := GetContact("44858144348@chatroom")
	if err != nil {
		t.Logf("GetContact info: %v", err)
	} else {
		t.Logf("Contact: %v (%v)", contact["username"], contact["displayName"])
	}
}
