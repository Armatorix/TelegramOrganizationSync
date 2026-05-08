package devserver

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Armatorix/TelegramOrganizationSync/internal/api"
)

func TestCreateOrGetIsIdempotentByChatID(t *testing.T) {
	s := NewStore()
	a := s.CreateOrGet(api.CreateChannelRequest{TelegramChatID: -1, Title: "first"})
	b := s.CreateOrGet(api.CreateChannelRequest{TelegramChatID: -1, Title: "second-attempt"})
	if a.ID != b.ID {
		t.Fatalf("expected same id on duplicate create, got %q and %q", a.ID, b.ID)
	}
	if a.SyncStatus != api.SyncStatusDisabled {
		t.Fatalf("expected new channel to start disabled, got %q", a.SyncStatus)
	}
}

func TestReconcileDiffs(t *testing.T) {
	cases := []struct {
		name     string
		expected []api.Member
		received []api.Member
		add      []int64
		remove   []int64
	}{
		{
			name:     "in sync",
			expected: members(1, 2, 3),
			received: members(1, 2, 3),
		},
		{
			name:     "drift in both directions",
			expected: members(1, 3, 4),
			received: members(1, 2, 3),
			add:      []int64{4},
			remove:   []int64{2},
		},
		{
			name:     "empty server expected wipes",
			received: members(1, 2),
			remove:   []int64{1, 2},
		},
		{
			name:     "empty telegram fills",
			expected: members(1, 2),
			add:      []int64{1, 2},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := NewStore()
			ch := s.CreateOrGet(api.CreateChannelRequest{TelegramChatID: -1, Title: c.name})
			s.SetExpected(ch.ID, c.expected)

			resp, ok := s.Reconcile(ch.ID, c.received)
			if !ok {
				t.Fatalf("Reconcile returned not-found for known channel")
			}
			if got := userIDs(resp.ToAdd); !equal(got, c.add) {
				t.Errorf("to_add: got %v, want %v", got, c.add)
			}
			if got := userIDs(resp.ToRemove); !equal(got, c.remove) {
				t.Errorf("to_remove: got %v, want %v", got, c.remove)
			}

			hist := s.History(ch.ID)
			if len(hist) != 1 {
				t.Fatalf("history len = %d, want 1", len(hist))
			}
		})
	}
}

func TestReconcileUnknownChannelReturnsNotFound(t *testing.T) {
	s := NewStore()
	if _, ok := s.Reconcile("does-not-exist", nil); ok {
		t.Fatal("expected ok=false for unknown channel")
	}
}

func members(ids ...int64) []api.Member {
	out := make([]api.Member, len(ids))
	for i, id := range ids {
		out[i] = api.Member{TelegramUserID: id}
	}
	return out
}

func userIDs(ms []api.Member) []int64 {
	if len(ms) == 0 {
		return nil
	}
	out := make([]int64, len(ms))
	for i, m := range ms {
		out[i] = m.TelegramUserID
	}
	slices.Sort(out)
	return out
}

func equal(a, b []int64) bool { return reflect.DeepEqual(a, b) }
