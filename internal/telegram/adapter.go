// Package telegram is the Telegram side of the world. It defines the
// Adapter interface used by the sync engine and provides two
// implementations: a JSON-file-backed fake (default) and a TDLib-backed
// real adapter (behind the `tdlib` build tag).
package telegram

import (
	"context"

	"github.com/Armatorix/TelegramOrganizationSync/internal/api"
)

type Adapter interface {
	ListMembers(ctx context.Context, chatID int64) ([]api.Member, error)
	AddMember(ctx context.Context, chatID, userID int64) error
	RemoveMember(ctx context.Context, chatID, userID int64) error
	SendDM(ctx context.Context, userID int64, text string) error

	// DiscoveredChannels emits a channel for every Telegram chat the
	// account is in that the engine should consider registering on the
	// server. The implementation is responsible for de-duplication.
	DiscoveredChannels() <-chan DiscoveredChannel

	Close() error
}

type DiscoveredChannel struct {
	TelegramChatID int64
	Title          string
}
