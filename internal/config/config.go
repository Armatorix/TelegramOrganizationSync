// Package config loads the client configuration from a YAML file plus
// environment-variable overrides. Env wins for secrets so they don't have
// to live on disk.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Mode string

const (
	ModeAuto   Mode = "auto"
	ModeManual Mode = "manual"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Mode     Mode           `yaml:"mode"`
	Telegram TelegramConfig `yaml:"telegram"`
	Sync     SyncConfig     `yaml:"sync"`
}

type ServerConfig struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

type TelegramConfig struct {
	// FakeStateFile, when set, selects the file-backed fake adapter instead
	// of TDLib. Useful for local dev without the C library installed.
	FakeStateFile string `yaml:"fake_state_file"`

	APIID       int32  `yaml:"api_id"`
	APIHash     string `yaml:"api_hash"`
	DatabaseDir string `yaml:"database_dir"`
	PhoneNumber string `yaml:"phone_number"`
	BotToken    string `yaml:"bot_token"`
}

type SyncConfig struct {
	Interval  time.Duration `yaml:"interval"`
	BatchSize int           `yaml:"batch_size"`
	DryRun    bool          `yaml:"dry_run"`
}

func Load(path string) (Config, error) {
	var cfg Config
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config: %w", err)
		}
	}
	applyEnv(&cfg)
	applyDefaults(&cfg)
	return cfg, validate(cfg)
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("TOS_SERVER_URL"); v != "" {
		cfg.Server.URL = v
	}
	if v := os.Getenv("TOS_SERVER_API_KEY"); v != "" {
		cfg.Server.APIKey = v
	}
	if v := os.Getenv("TOS_MODE"); v != "" {
		cfg.Mode = Mode(v)
	}
	if v := os.Getenv("TOS_TG_FAKE_STATE_FILE"); v != "" {
		cfg.Telegram.FakeStateFile = v
	}
	if v := os.Getenv("TOS_TG_API_ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			cfg.Telegram.APIID = int32(n)
		}
	}
	if v := os.Getenv("TOS_TG_API_HASH"); v != "" {
		cfg.Telegram.APIHash = v
	}
	if v := os.Getenv("TOS_TG_DB_DIR"); v != "" {
		cfg.Telegram.DatabaseDir = v
	}
	if v := os.Getenv("TOS_TG_PHONE"); v != "" {
		cfg.Telegram.PhoneNumber = v
	}
	if v := os.Getenv("TOS_TG_BOT_TOKEN"); v != "" {
		cfg.Telegram.BotToken = v
	}
	if v := os.Getenv("TOS_SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Sync.Interval = d
		}
	}
	if v := os.Getenv("TOS_DRY_RUN"); v != "" {
		cfg.Sync.DryRun = v == "1" || v == "true"
	}
}

func applyDefaults(cfg *Config) {
	if cfg.Mode == "" {
		cfg.Mode = ModeManual
	}
	if cfg.Sync.Interval == 0 {
		cfg.Sync.Interval = 5 * time.Minute
	}
	if cfg.Sync.BatchSize == 0 {
		cfg.Sync.BatchSize = 200
	}
}

func validate(cfg Config) error {
	if cfg.Server.URL == "" {
		return errors.New("server.url is required")
	}
	if cfg.Server.APIKey == "" {
		return errors.New("server.api_key is required")
	}
	if cfg.Mode != ModeAuto && cfg.Mode != ModeManual {
		return fmt.Errorf("mode must be %q or %q, got %q", ModeAuto, ModeManual, cfg.Mode)
	}
	if cfg.Telegram.FakeStateFile == "" {
		// Real TDLib path — minimal sanity check.
		if cfg.Telegram.APIID == 0 || cfg.Telegram.APIHash == "" {
			return errors.New("telegram.api_id and telegram.api_hash are required when fake_state_file is unset")
		}
		if cfg.Telegram.PhoneNumber == "" && cfg.Telegram.BotToken == "" {
			return errors.New("either telegram.phone_number or telegram.bot_token must be set")
		}
		if cfg.Telegram.PhoneNumber != "" && cfg.Telegram.BotToken != "" {
			return errors.New("telegram.phone_number and telegram.bot_token are mutually exclusive")
		}
	}
	return nil
}
