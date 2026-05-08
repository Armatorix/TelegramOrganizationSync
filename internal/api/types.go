// Package api holds the wire types shared between the client (cmd/tos)
// and the org server (cmd/devserver, and any production server).
package api

const (
	SyncStatusEnabled  = "enabled"
	SyncStatusDisabled = "disabled"
)

type Channel struct {
	ID             string   `json:"id"`
	TelegramChatID int64    `json:"telegram_chat_id"`
	Title          string   `json:"title"`
	SyncStatus     string   `json:"sync_status"`
	Manager        *Manager `json:"manager,omitempty"`
}

type Manager struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Name           string `json:"name"`
}

type Member struct {
	TelegramUserID int64  `json:"telegram_user_id"`
	Name           string `json:"name"`
}

type CreateChannelRequest struct {
	TelegramChatID int64  `json:"telegram_chat_id"`
	Title          string `json:"title"`
}

type ReconcileRequest struct {
	Members []Member `json:"members"`
}

type ReconcileResponse struct {
	ToAdd    []Member `json:"to_add"`
	ToRemove []Member `json:"to_remove"`
}

// Problem is the error envelope (RFC 7807 subset).
type Problem struct {
	Type   string `json:"type,omitempty"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func (p *Problem) Error() string {
	if p.Detail != "" {
		return p.Title + ": " + p.Detail
	}
	return p.Title
}
