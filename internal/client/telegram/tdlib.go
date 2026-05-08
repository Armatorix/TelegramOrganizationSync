//go:build tdlib

// Real TDLib adapter. Build with `-tags tdlib` after installing the TDLib
// C library and pulling github.com/zelenin/go-tdlib via `go get`.
//
// This file is intentionally a thin scaffold: TDLib semantics (auth flow,
// member paging, error mapping) are non-trivial and deserve their own
// tests against a real test account, which is out of scope for the
// initial design pass.
//
// Wiring checklist:
//  1. go get github.com/zelenin/go-tdlib/client
//  2. Install TDLib system-wide (https://tdlib.github.io/td/build.html).
//  3. Replace the method bodies below with calls to the
//     zelenin/go-tdlib client and remove errNotImplemented.

package telegram

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Armatorix/TelegramOrganizationSync/internal/api"
	"github.com/Armatorix/TelegramOrganizationSync/internal/client/config"
)

var errNotImplemented = errors.New("tdlib adapter not yet wired up; see internal/client/telegram/tdlib.go")

type TDLib struct {
	cfg        config.TelegramConfig
	log        *slog.Logger
	discovered chan DiscoveredChannel
}

func NewTDLib(cfg config.TelegramConfig, log *slog.Logger) (*TDLib, error) {
	return nil, errNotImplemented
}

func (t *TDLib) Close() error                                 { return nil }
func (t *TDLib) DiscoveredChannels() <-chan DiscoveredChannel { return t.discovered }

func (t *TDLib) ListMembers(context.Context, int64) ([]api.Member, error) {
	return nil, errNotImplemented
}
func (t *TDLib) AddMember(context.Context, int64, int64) error    { return errNotImplemented }
func (t *TDLib) RemoveMember(context.Context, int64, int64) error { return errNotImplemented }
func (t *TDLib) SendDM(context.Context, int64, string) error      { return errNotImplemented }
