package devserver

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Armatorix/TelegramOrganizationSync/internal/api"
)

//go:embed ui.html
var uiFS embed.FS

type Server struct {
	store  *Store
	apiKey string
	log    *slog.Logger
	tmpl   *template.Template
}

func New(store *Store, apiKey string, log *slog.Logger) (*Server, error) {
	tmpl, err := template.ParseFS(uiFS, "ui.html")
	if err != nil {
		return nil, fmt.Errorf("parse ui template: %w", err)
	}
	return &Server{store: store, apiKey: apiKey, log: log, tmpl: tmpl}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Spec API
	mux.HandleFunc("GET /api/v1/channels", s.requireAPIKey(s.listChannels))
	mux.HandleFunc("POST /api/v1/channels", s.requireAPIKey(s.createChannel))
	mux.HandleFunc("POST /api/v1/channels/{id}/members:reconcile", s.requireAPIKey(s.reconcile))

	// Admin UI (no api key — local dev)
	mux.HandleFunc("GET /", s.indexPage)
	mux.HandleFunc("POST /admin/channels", s.adminCreate)
	mux.HandleFunc("POST /admin/channels/{id}/sync", s.adminSetSync)
	mux.HandleFunc("POST /admin/channels/{id}/manager", s.adminSetManager)
	mux.HandleFunc("POST /admin/channels/{id}/expected", s.adminSetExpected)
	mux.HandleFunc("POST /admin/channels/{id}/delete", s.adminDelete)

	return mux
}

// --- API ---

func (s *Server) listChannels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.store.List())
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	var req api.CreateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid body", err.Error())
		return
	}
	if req.TelegramChatID == 0 {
		writeProblem(w, http.StatusBadRequest, "telegram_chat_id required", "")
		return
	}
	ch := s.store.CreateOrGet(req)
	writeJSON(w, http.StatusOK, ch)
}

func (s *Server) reconcile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req api.ReconcileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid body", err.Error())
		return
	}
	resp, ok := s.store.Reconcile(id, req.Members)
	if !ok {
		writeProblem(w, http.StatusNotFound, "channel not found", "id="+id)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Admin UI ---

type pageData struct {
	APIKey   string
	Channels []channelView
}

type channelView struct {
	api.Channel
	Expected string
	History  []Reconcil
}

func (s *Server) indexPage(w http.ResponseWriter, _ *http.Request) {
	chs := s.store.List()
	views := make([]channelView, 0, len(chs))
	for _, ch := range chs {
		views = append(views, channelView{
			Channel:  ch,
			Expected: formatMembers(s.store.Expected(ch.ID)),
			History:  s.store.History(ch.ID),
		})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.Execute(w, pageData{APIKey: s.apiKey, Channels: views}); err != nil {
		s.log.Warn("template execute failed", "err", err)
	}
}

func (s *Server) adminCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	chatID, err := strconv.ParseInt(r.Form.Get("telegram_chat_id"), 10, 64)
	if err != nil {
		http.Error(w, "telegram_chat_id must be int64", http.StatusBadRequest)
		return
	}
	s.store.CreateOrGet(api.CreateChannelRequest{
		TelegramChatID: chatID,
		Title:          r.Form.Get("title"),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) adminSetSync(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.store.SetSyncStatus(id, r.Form.Get("status")) {
		http.Error(w, "invalid channel or status", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) adminSetManager(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	uidStr := strings.TrimSpace(r.Form.Get("telegram_user_id"))
	name := strings.TrimSpace(r.Form.Get("name"))
	var mgr *api.Manager
	if uidStr != "" || name != "" {
		uid, err := strconv.ParseInt(uidStr, 10, 64)
		if err != nil {
			http.Error(w, "telegram_user_id must be int64", http.StatusBadRequest)
			return
		}
		mgr = &api.Manager{TelegramUserID: uid, Name: name}
	}
	if !s.store.SetManager(id, mgr) {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) adminSetExpected(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	members, err := parseMembers(r.Form.Get("members"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.store.SetExpected(id, members) {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) adminDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.store.Delete(id) {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// --- helpers ---

func (s *Server) requireAPIKey(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, prefix) || auth[len(prefix):] != s.apiKey {
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "missing or invalid api key")
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.Problem{
		Title:  title,
		Status: status,
		Detail: detail,
	})
}

func formatMembers(ms []api.Member) string {
	var b strings.Builder
	for _, m := range ms {
		fmt.Fprintf(&b, "%d %s\n", m.TelegramUserID, m.Name)
	}
	return b.String()
}

func parseMembers(raw string) ([]api.Member, error) {
	var out []api.Member
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		idStr, name, ok := strings.Cut(line, " ")
		if !ok {
			return nil, fmt.Errorf("line %q: expected `<user_id> <name>`", line)
		}
		uid, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("line %q: bad user id: %w", line, err)
		}
		out = append(out, api.Member{
			TelegramUserID: uid,
			Name:           strings.TrimSpace(name),
		})
	}
	return out, scanner.Err()
}
