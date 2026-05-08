package syncengine

import (
	"strings"
	"testing"

	"github.com/Armatorix/TelegramOrganizationSync/internal/api"
)

func TestDangerousRemoval(t *testing.T) {
	cases := []struct {
		name      string
		current   int
		remove    int
		dangerous bool
	}{
		{"empty channel", 0, 0, false},
		{"remove none", 10, 0, false},
		{"remove half exactly", 10, 5, false},
		{"remove just over half", 10, 6, true},
		{"remove all", 10, 10, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dangerousRemoval(c.current, c.remove); got != c.dangerous {
				t.Errorf("dangerousRemoval(%d, %d) = %v, want %v",
					c.current, c.remove, got, c.dangerous)
			}
		})
	}
}

func TestRenderManagerMessage(t *testing.T) {
	ch := api.Channel{Title: "engineering"}
	diff := api.ReconcileResponse{
		ToAdd:    []api.Member{{TelegramUserID: 1, Name: "Alice"}},
		ToRemove: []api.Member{{TelegramUserID: 2, Name: "Bob"}},
	}
	msg := renderManagerMessage(ch, diff, "")

	for _, want := range []string{"engineering", "Alice", "Bob", "+ Alice", "- Bob"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected message to contain %q, got:\n%s", want, msg)
		}
	}
}

func TestRenderManagerMessageWithPrefix(t *testing.T) {
	msg := renderManagerMessage(api.Channel{Title: "x"}, api.ReconcileResponse{}, "WARN: ")
	if !strings.HasPrefix(msg, "WARN: ") {
		t.Errorf("expected prefix, got %q", msg)
	}
}
