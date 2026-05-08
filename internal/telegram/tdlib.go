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
//  1. `go get github.com/zelenin/go-tdlib/client`
//  2. Install TDLib system-wide (https://tdlib.github.io/td/build.html).
//  3. Replace the bodies below with calls to the zelenin/go-tdlib client.

package telegram

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Armatorix/TelegramOrganizationSync/internal/api"
	"github.com/Armatorix/TelegramOrganizationSync/internal/config"
)

type TDLib struct {
	cfg        config.TelegramConfig
	log        *slog.Logger
	discovered chan DiscoveredChannel
}

func NewTDLib(cfg config.TelegramConfig, log *slog.Logger) (*TDLib, error) {
	return &TDLib{
		cfg:        cfg,
		log:        log,
		discovered: make(chan DiscoveredChannel, 32),
	}, errors.New("tdlib adapter not yet wired up; see internal/telegram/tdlib.go")
}

func (t *TDLib) Close() error                                   { return nil }
func (t *TDLib) DiscoveredChannels() <-chan DiscoveredChannel    { return t.discovered }
func (t *TDLib) ListMembers(context.Context, int64) ([]api.Member, error) {
	return nil, errors.New("tdlib: ListMembers not implemented")
}
func (t *TDLib) AddMember(context.Context, int64, int64) error {
	return errors.New("tdlib: AddMember not implemented")
}
func (t *TDLib) RemoveMember(context.Context, int64, int64) error {
	return errors.New("tdlib: RemoveMember not implemented")
}
func (t *TDLib) SendDM(context.Context, int64, string) error {
	return errors.New("tdlib: SendDM not implemented")
}
