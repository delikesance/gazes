package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
)

const (
	tokenNameMax      = 60
	tokenMaxActive    = 20
	tokenDefaultHours = 24 * 30
	tokenMaxHours     = 24 * 365
)

func (s *Service) mountTokens(r chi.Router) {
	// Creating and revoking a token is for a human administrator: an admin session with the CSRF header.
	// A Bearer token is refused on these routes whatever its scopes, so a token can never mint another.
	r.With(s.sessionOnly).Post("/tokens", s.handleTokenCreate)
	r.With(s.sessionOnly).Delete("/tokens/{id}", s.handleTokenRevoke)
}

// handleTokenCreate: POST /tokens {name, scopes, ttl_hours}. The plain token is in the answer ONCE (never
// stored: only its hash is) so the panel can show the ready-to-paste `claude mcp add` command.
func (s *Service) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string   `json:"name"`
		Scopes   []string `json:"scopes"`
		TTLHours *int     `json:"ttl_hours"`
	}
	if !issueDecode(w, r, &in) {
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > tokenNameMax || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		writeAPIError(w, http.StatusBadRequest, "bad_name", "name is required (at most 60 characters, no control characters)")
		return
	}
	scopes, err := ParseScopes(in.Scopes)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_scopes", err.Error())
		return
	}
	hours := tokenDefaultHours
	if in.TTLHours != nil {
		hours = *in.TTLHours
	}
	if hours < 1 || hours > tokenMaxHours {
		writeAPIError(w, http.StatusBadRequest, "bad_ttl", "ttl_hours must be between 1 and 8760")
		return
	}
	toks, err := s.store.ListTokens(r.Context())
	if err != nil {
		pbServerError(w, err)
		return
	}
	active, now := 0, s.now()
	for _, t := range toks {
		if t.Status(now) == "active" {
			active++
		}
	}
	if active >= tokenMaxActive {
		writeAPIError(w, http.StatusConflict, "too_many_tokens", "20 active tokens already: revoke one first")
		return
	}
	plain, id, err := s.store.CreateToken(r.Context(), name, scopes, time.Duration(hours)*time.Hour)
	if err != nil {
		pbServerError(w, err)
		return
	}
	actor, _ := s.actorOf(r)
	s.opsAudit(r.Context(), 0, "token:create", "by="+actor+" id="+strconv.FormatInt(id, 10)+" scopes="+strings.Join(scopes, ","), "ok", 0)
	writeJSON(w, http.StatusCreated, envelope{GeneratedAt: now.UTC().Format(time.RFC3339), Data: map[string]any{
		"token":      plain,
		"id":         id,
		"name":       name,
		"scopes":     scopes,
		"expires_at": now.Add(time.Duration(hours) * time.Hour).UTC().Format(time.RFC3339),
	}})
}

// handleTokenRevoke: DELETE /tokens/{id}. 204 when revoked (or already revoked), 404 for an unknown id.
func (s *Service) handleTokenRevoke(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown token")
		return
	}
	toks, err := s.store.ListTokens(r.Context())
	if err != nil {
		pbServerError(w, err)
		return
	}
	found := false
	for _, t := range toks {
		if t.ID == id {
			found = true
		}
	}
	if !found {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown token")
		return
	}
	if err := s.store.RevokeToken(r.Context(), id); err != nil {
		pbServerError(w, err)
		return
	}
	actor, _ := s.actorOf(r)
	s.opsAudit(r.Context(), 0, "token:revoke", "by="+actor+" id="+strconv.FormatInt(id, 10), "ok", 0)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
