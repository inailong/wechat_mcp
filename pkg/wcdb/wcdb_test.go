//go:build (linux && cgo) || windows

package wcdb

import (
	"os"
	"testing"

	"github.com/inailong/wechat_mcp/pkg/wechat"
)

func TestNativeWCDBDirect(t *testing.T) {
	info, err := wechat.DetectAccount()
	if err != nil || info == nil || info.SessionDb == "" || info.DecryptKey == "" {
		t.Skip("session.db or decrypt key not detected, skipping TestNativeWCDBDirect")
	}

	sessionDb := info.SessionDb
	realKey := info.DecryptKey

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
	t.Logf("Got %d sessions directly via native driver!", len(sessions))
	if len(sessions) == 0 {
		t.Logf("Expected sessions > 0, but got 0")
		return
	}

	first := sessions[0]
	t.Logf("First session: %v (%v)", first["username"], first["displayName"])

	firstUser, _ := first["username"].(string)
	if firstUser != "" {
		msgs, err := GetMessages(firstUser, 3, 0)
		if err != nil {
			t.Logf("GetMessages warning: %v", err)
		} else {
			t.Logf("Got %d messages directly from message DB!", len(msgs))
			if len(msgs) > 0 {
				t.Logf("Latest msg: %v", msgs[0]["message_content"])
			}
		}

		contact, err := GetContact(firstUser)
		if err != nil {
			t.Logf("GetContact info: %v", err)
		} else {
			t.Logf("Contact: %v (%v)", contact["username"], contact["displayName"])
		}
	}
}
