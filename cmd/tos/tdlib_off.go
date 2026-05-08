//go:build !tdlib

package main

import (
	"errors"
	"log/slog"

	"github.com/Armatorix/TelegramOrganizationSync/internal/config"
	"github.com/Armatorix/TelegramOrganizationSync/internal/telegram"
)

func newTDLib(_ config.Config, _ *slog.Logger) (telegram.Adapter, error) {
	return nil, errors.New("TDLib adapter not compiled in; rebuild with `-tags tdlib` or set telegram.fake_state_file for local dev")
}
