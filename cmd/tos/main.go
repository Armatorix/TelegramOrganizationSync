// Command tos is the TelegramOrganizationSync client. It logs into
// Telegram (via the fake adapter or TDLib), then ticks the sync engine
// against the configured org server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Armatorix/TelegramOrganizationSync/internal/config"
	"github.com/Armatorix/TelegramOrganizationSync/internal/server"
	syncpkg "github.com/Armatorix/TelegramOrganizationSync/internal/sync"
	"github.com/Armatorix/TelegramOrganizationSync/internal/telegram"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tos:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "path to config yaml (env vars override)")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	srv, err := server.New(cfg.Server.URL, cfg.Server.APIKey)
	if err != nil {
		return fmt.Errorf("server client: %w", err)
	}

	tg, err := newTelegram(cfg, log)
	if err != nil {
		return fmt.Errorf("telegram adapter: %w", err)
	}
	defer tg.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	engine := syncpkg.New(cfg, srv, tg, log)
	if err := engine.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	log.Info("shutting down")
	return nil
}

func newTelegram(cfg config.Config, log *slog.Logger) (telegram.Adapter, error) {
	if cfg.Telegram.FakeStateFile != "" {
		log.Info("using fake telegram adapter", "state_file", cfg.Telegram.FakeStateFile)
		return telegram.NewFake(cfg.Telegram.FakeStateFile, log)
	}
	return newTDLib(cfg, log)
}
