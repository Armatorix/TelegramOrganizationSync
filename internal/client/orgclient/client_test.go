package orgclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Armatorix/TelegramOrganizationSync/internal/api"
)

func TestClientSetsBearerAuth(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c, err := New(srv.URL, "topsecret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListChannels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if seen != "Bearer topsecret" {
		t.Errorf("Authorization header = %q, want Bearer topsecret", seen)
	}
}

func TestReconcileRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/members:reconcile") {
			t.Errorf("path = %s, want suffix /members:reconcile", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req api.ReconcileRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		if len(req.Members) != 1 || req.Members[0].TelegramUserID != 42 {
			t.Errorf("unexpected members: %+v", req.Members)
		}
		_ = json.NewEncoder(w).Encode(api.ReconcileResponse{
			ToAdd: []api.Member{{TelegramUserID: 7, Name: "Carol"}},
		})
	}))
	defer srv.Close()

	c, err := New(srv.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Reconcile(context.Background(), "ch_xyz", api.ReconcileRequest{
		Members: []api.Member{{TelegramUserID: 42, Name: "Alice"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ToAdd) != 1 || resp.ToAdd[0].Name != "Carol" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestProblemErrorIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(api.Problem{Title: "bad input", Detail: "missing field"})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "key")
	_, err := c.ListChannels(context.Background())
	if err == nil {
		t.Fatal("expected error on 400")
	}
	prob, ok := err.(*api.Problem)
	if !ok {
		t.Fatalf("expected *api.Problem, got %T: %v", err, err)
	}
	if prob.Status != http.StatusBadRequest || prob.Title != "bad input" {
		t.Errorf("unexpected problem: %+v", prob)
	}
}
