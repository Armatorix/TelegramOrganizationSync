// Package devserver is an in-memory implementation of the org server,
// for local development. It serves the three API endpoints documented in
// DESIGN.md plus a small admin UI for poking at state.
package devserver

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/Armatorix/TelegramOrganizationSync/internal/api"
)

const historyLimit = 20

type Store struct {
	mu       sync.RWMutex
	channels map[string]*record    // id -> record
	byChat   map[int64]string      // telegram_chat_id -> id (idempotency)
	history  map[string][]Reconcil // id -> recent reconcile snapshots
}

type record struct {
	Channel  api.Channel
	Expected []api.Member // what membership should look like
}

type Reconcil struct {
	At       time.Time
	Received []api.Member
	ToAdd    []api.Member
	ToRemove []api.Member
}

func NewStore() *Store {
	return &Store{
		channels: map[string]*record{},
		byChat:   map[int64]string{},
		history:  map[string][]Reconcil{},
	}
}

func (s *Store) List() []api.Channel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]api.Channel, 0, len(s.channels))
	for _, r := range s.channels {
		out = append(out, r.Channel)
	}
	return out
}

func (s *Store) Get(id string) (api.Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.channels[id]
	if !ok {
		return api.Channel{}, false
	}
	return r.Channel, true
}

// CreateOrGet creates a new channel with sync_status=disabled, or returns
// the existing channel if telegram_chat_id is already known. Idempotent.
func (s *Store) CreateOrGet(req api.CreateChannelRequest) api.Channel {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.byChat[req.TelegramChatID]; ok {
		return s.channels[id].Channel
	}
	id := newID()
	r := &record{
		Channel: api.Channel{
			ID:             id,
			TelegramChatID: req.TelegramChatID,
			Title:          req.Title,
			SyncStatus:     api.SyncStatusDisabled,
		},
	}
	s.channels[id] = r
	s.byChat[req.TelegramChatID] = id
	return r.Channel
}

func (s *Store) SetSyncStatus(id, status string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.channels[id]
	if !ok {
		return false
	}
	if status != api.SyncStatusEnabled && status != api.SyncStatusDisabled {
		return false
	}
	r.Channel.SyncStatus = status
	return true
}

func (s *Store) SetManager(id string, m *api.Manager) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.channels[id]
	if !ok {
		return false
	}
	r.Channel.Manager = m
	return true
}

func (s *Store) SetExpected(id string, members []api.Member) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.channels[id]
	if !ok {
		return false
	}
	r.Expected = members
	return true
}

func (s *Store) Expected(id string) []api.Member {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.channels[id]
	if !ok {
		return nil
	}
	return append([]api.Member(nil), r.Expected...)
}

func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.channels[id]
	if !ok {
		return false
	}
	delete(s.channels, id)
	delete(s.byChat, r.Channel.TelegramChatID)
	delete(s.history, id)
	return true
}

// Reconcile compares the received membership against the expected list
// and returns the diff. Records the operation in the channel's history.
func (s *Store) Reconcile(id string, received []api.Member) (api.ReconcileResponse, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.channels[id]
	if !ok {
		return api.ReconcileResponse{}, false
	}

	expected := r.Expected
	expectedByID := indexByID(expected)
	receivedByID := indexByID(received)

	var toAdd, toRemove []api.Member
	for _, m := range expected {
		if _, ok := receivedByID[m.TelegramUserID]; !ok {
			toAdd = append(toAdd, m)
		}
	}
	for _, m := range received {
		if _, ok := expectedByID[m.TelegramUserID]; !ok {
			toRemove = append(toRemove, m)
		}
	}

	entry := Reconcil{
		At:       time.Now().UTC(),
		Received: append([]api.Member(nil), received...),
		ToAdd:    append([]api.Member(nil), toAdd...),
		ToRemove: append([]api.Member(nil), toRemove...),
	}
	hist := append([]Reconcil{entry}, s.history[id]...)
	if len(hist) > historyLimit {
		hist = hist[:historyLimit]
	}
	s.history[id] = hist

	return api.ReconcileResponse{ToAdd: toAdd, ToRemove: toRemove}, true
}

func (s *Store) History(id string) []Reconcil {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Reconcil(nil), s.history[id]...)
}

func indexByID(ms []api.Member) map[int64]struct{} {
	m := make(map[int64]struct{}, len(ms))
	for _, x := range ms {
		m[x.TelegramUserID] = struct{}{}
	}
	return m
}

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "ch_" + hex.EncodeToString(b[:])
}
