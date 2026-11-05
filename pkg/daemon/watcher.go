package daemon

import (
	"context"
	"strings"
	"time"

	"github.com/inailong/wechat_mcp/pkg/bridge"
	"github.com/inailong/wechat_mcp/pkg/wechat"
)

type InboundMessage struct {
	ID         string   `json:"id"`
	Sender     string   `json:"sender"`
	SessionID  string   `json:"sessionId"`
	Content    string   `json:"content"`
	ImagePaths []string `json:"imagePaths,omitempty"`
	Timestamp  int64    `json:"timestamp"`
}

type MessageSource interface {
	Messages() <-chan InboundMessage
}

type WCDBWatcher struct {
	bClient    *bridge.Client
	msgChan    chan InboundMessage
	selfWxID   string
	lastSeen   int64
	seenMsgIDs map[string]int64
}

func NewWCDBWatcher(bClient *bridge.Client) *WCDBWatcher {
	selfWxID := ""
	if info, err := wechat.DetectAccount(); err == nil && info != nil {
		selfWxID = info.WxID
	}

	return &WCDBWatcher{
		bClient:    bClient,
		msgChan:    make(chan InboundMessage, 50),
		selfWxID:   selfWxID,
		lastSeen:   time.Now().Unix(),
		seenMsgIDs: make(map[string]int64),
	}
}

func (w *WCDBWatcher) Messages() <-chan InboundMessage {
	return w.msgChan
}

func (w *WCDBWatcher) Start(ctx context.Context) error {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.pollNewMessages(ctx)
		}
	}
}

func (w *WCDBWatcher) pollNewMessages(ctx context.Context) {
	if w.bClient == nil {
		return
	}

	now := time.Now().Unix()
	// Allow 5-second overlap window to absorb clock jitter and database commit latency
	querySince := w.lastSeen - 5
	if querySince < 0 {
		querySince = 0
	}

	sessions, err := w.bClient.ListSessions(ctx, "", 20, 0)
	if err != nil || sessions == nil {
		return
	}

	for _, sess := range sessions.Sessions {
		if sess.LastTimestamp < querySince {
			continue
		}

		msgs, err := w.bClient.ListMessages(ctx, sess.SessionID, 10, 0, "desc", querySince, now+10)
		if err != nil || msgs == nil {
			continue
		}

		for _, m := range msgs.Messages {
			if w.seenMsgIDs[m.ID] > 0 {
				continue
			}
			w.seenMsgIDs[m.ID] = m.Timestamp

			// Skip self messages
			if w.selfWxID != "" && (m.Sender == w.selfWxID || strings.HasPrefix(m.Sender, w.selfWxID)) {
				continue
			}

			// Clean content
			content := strings.TrimSpace(m.Content)
			if content == "" {
				continue
			}

			var imgPaths []string
			if m.FilePath != "" && isImageFile(m.FilePath) {
				imgPaths = append(imgPaths, m.FilePath)
			}

			inbound := InboundMessage{
				ID:         m.ID,
				Sender:     m.Sender,
				SessionID:  sess.SessionID,
				Content:    content,
				ImagePaths: imgPaths,
				Timestamp:  m.Timestamp,
			}

			select {
			case w.msgChan <- inbound:
			default:
			}
		}
	}

	// Prune old seen IDs older than 10 minutes to prevent memory leak
	if len(w.seenMsgIDs) > 200 {
		cutoff := now - 600
		for id, ts := range w.seenMsgIDs {
			if ts < cutoff {
				delete(w.seenMsgIDs, id)
			}
		}
	}

	w.lastSeen = now
}

func isImageFile(path string) bool {
	p := strings.ToLower(path)
	return strings.HasSuffix(p, ".jpg") || strings.HasSuffix(p, ".jpeg") ||
		strings.HasSuffix(p, ".png") || strings.HasSuffix(p, ".webp")
}
