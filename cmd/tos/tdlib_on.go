//go:build tdlib

package main

import (
	"log/slog"

	"github.com/Armatorix/TelegramOrganizationSync/internal/client/config"
	"github.com/Armatorix/TelegramOrganizationSync/internal/client/telegram"
)

func newTDLib(cfg config.Config, log *slog.Logger) (telegram.Adapter, error) {
	return telegram.NewTDLib(cfg.Telegram, log)
}
