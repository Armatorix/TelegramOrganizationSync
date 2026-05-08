package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/Armatorix/TelegramOrganizationSync/internal/api"
)

// Fake is a JSON-file-backed Telegram adapter for local development and
// tests. State lives entirely in the file on disk; every call reads and
// (for mutations) atomically rewrites it. A background goroutine polls
// the file for new chats and surfaces them through DiscoveredChannels.
type Fake struct {
	path string
	log  *slog.Logger

	mu sync.Mutex // serializes file rewrites within this process

	discovered      chan DiscoveredChannel
	stopCh          chan struct{}
	seenDiscoveries map[int64]struct{}
}

type FakeState struct {
	Channels []FakeChannel `json:"channels"`
	DMs      []FakeDM      `json:"dms"`
}

type FakeChannel struct {
	TelegramChatID int64        `json:"telegram_chat_id"`
	Title          string       `json:"title"`
	Members        []api.Member `json:"members"`
}

type FakeDM struct {
	At             time.Time `json:"at"`
	TelegramUserID int64     `json:"telegram_user_id"`
	Text           string    `json:"text"`
}

func NewFake(path string, log *slog.Logger) (*Fake, error) {
	f := &Fake{
		path:            path,
		log:             log,
		discovered:      make(chan DiscoveredChannel, 32),
		stopCh:          make(chan struct{}),
		seenDiscoveries: make(map[int64]struct{}),
	}
	if _, err := f.read(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := f.write(FakeState{}); err != nil {
			return nil, err
		}
	}
	go f.discoverLoop()
	return f, nil
}

func (f *Fake) Close() error {
	close(f.stopCh)
	return nil
}

func (f *Fake) DiscoveredChannels() <-chan DiscoveredChannel { return f.discovered }

func (f *Fake) ListMembers(_ context.Context, chatID int64) ([]api.Member, error) {
	st, err := f.read()
	if err != nil {
		return nil, err
	}
	for _, ch := range st.Channels {
		if ch.TelegramChatID == chatID {
			return slices.Clone(ch.Members), nil
		}
	}
	return nil, fmt.Errorf("fake: chat %d not found", chatID)
}

func (f *Fake) AddMember(_ context.Context, chatID, userID int64) error {
	return f.mutate(func(st *FakeState) error {
		ch := f.findChannel(st, chatID)
		if ch == nil {
			return fmt.Errorf("fake: chat %d not found", chatID)
		}
		for _, m := range ch.Members {
			if m.TelegramUserID == userID {
				return nil // already a member, idempotent
			}
		}
		ch.Members = append(ch.Members, api.Member{
			TelegramUserID: userID,
			Name:           fmt.Sprintf("user-%d", userID),
		})
		f.log.Info("fake telegram add member", "chat_id", chatID, "user_id", userID)
		return nil
	})
}

func (f *Fake) RemoveMember(_ context.Context, chatID, userID int64) error {
	return f.mutate(func(st *FakeState) error {
		ch := f.findChannel(st, chatID)
		if ch == nil {
			return fmt.Errorf("fake: chat %d not found", chatID)
		}
		ch.Members = slices.DeleteFunc(ch.Members, func(m api.Member) bool {
			return m.TelegramUserID == userID
		})
		f.log.Info("fake telegram remove member", "chat_id", chatID, "user_id", userID)
		return nil
	})
}

func (f *Fake) SendDM(_ context.Context, userID int64, text string) error {
	return f.mutate(func(st *FakeState) error {
		st.DMs = append(st.DMs, FakeDM{
			At:             time.Now().UTC(),
			TelegramUserID: userID,
			Text:           text,
		})
		f.log.Info("fake telegram send dm", "user_id", userID, "len", len(text))
		return nil
	})
}

func (f *Fake) findChannel(st *FakeState, chatID int64) *FakeChannel {
	for i := range st.Channels {
		if st.Channels[i].TelegramChatID == chatID {
			return &st.Channels[i]
		}
	}
	return nil
}

func (f *Fake) mutate(fn func(*FakeState) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := f.read()
	if err != nil {
		return err
	}
	if err := fn(&st); err != nil {
		return err
	}
	return f.write(st)
}

func (f *Fake) read() (FakeState, error) {
	var st FakeState
	raw, err := os.ReadFile(f.path)
	if err != nil {
		return st, err
	}
	if len(raw) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, fmt.Errorf("parse fake state: %w", err)
	}
	return st, nil
}

func (f *Fake) write(st FakeState) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

func (f *Fake) discoverLoop() {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-f.stopCh:
			return
		case <-tick.C:
		}
		st, err := f.read()
		if err != nil {
			f.log.Warn("fake telegram: read state failed", "err", err)
			continue
		}
		for _, ch := range st.Channels {
			if _, ok := f.seenDiscoveries[ch.TelegramChatID]; ok {
				continue
			}
			f.seenDiscoveries[ch.TelegramChatID] = struct{}{}
			select {
			case f.discovered <- DiscoveredChannel{
				TelegramChatID: ch.TelegramChatID,
				Title:          ch.Title,
			}:
			default:
				// buffer full, will retry next tick
				delete(f.seenDiscoveries, ch.TelegramChatID)
			}
		}
	}
}
