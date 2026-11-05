package actuator

import (
	"context"
	"errors"
)

var (
	// ErrEmptyRecipient indicates that the recipient is empty or contains only whitespace.
	ErrEmptyRecipient = errors.New("recipient cannot be empty")
	// ErrEmptyContent indicates that the message content is empty or contains only whitespace.
	ErrEmptyContent = errors.New("content cannot be empty")
	// ErrWeChatNotFound indicates that no visible WeChat window was found.
	ErrWeChatNotFound = errors.New("wechat window not found")
	// ErrRecipientMismatch indicates that active chat header does not match target recipient.
	ErrRecipientMismatch = errors.New("active chat title does not match target recipient")
)

// Actuator defines the cross-platform GUI automation interface for WeChat.
type Actuator interface {
	SendMessage(ctx context.Context, recipient, content string) error
	SetDryRun(dryRun bool)
	IsDryRun() bool
}
