package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// SetOnAccountDeleted registers a callback run after an account is erased, so data kept in
// other databases (donations) can drop its link to the user.
func (s *Service) SetOnAccountDeleted(fn func(ctx context.Context, userID int64)) { s.onDelete = fn }

func (s *Service) clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.secure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

// DeleteAccount erases the signed-in account after re-checking its password (sent sealed, like login).
func (s *Service) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	plain, ok := s.decode(w, r, "delete-account")
	if !ok {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if json.Unmarshal(plain, &body) != nil {
		fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	key := "delete|" + s.clientIP(r)
	if s.throttled(w, key, 5, 15*time.Minute) {
		return
	}
	if !s.keys.verifyPassword(body.Password, u.PassHash) {
		fail(w, http.StatusForbidden, "invalid_credentials")
		return
	}
	if err := s.store.DeleteUser(u.ID); err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	if s.onDelete != nil {
		s.onDelete(r.Context(), u.ID)
	}
	s.clearCookie(w, r)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

// Export returns everything stored about the signed-in account as a JSON download.
func (s *Service) Export(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	v := s.view(u)
	progress, err1 := s.store.ListProgress(u.ID)
	history, err2 := s.store.ListWatchSessions(u.ID, 0, 100000)
	hidden, err3 := s.store.ListHidden(u.ID)
	watchlist, err4 := s.store.ListWatchlist(u.ID)
	notes, err5 := s.store.ListNotes(u.ID)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="gazes-export.json"`)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"pseudo": v.Pseudo, "email": v.Email, "exported_at": s.now().Unix(),
		"progress": progress, "history": history, "hidden": hidden, "watchlist": watchlist, "notes": notes,
	})
}

// ListSessions returns the devices signed in to the account.
func (s *Service) ListSessions(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	current := ""
	if c, err := r.Cookie(cookieName); err == nil {
		current = hashToken(c.Value)
	}
	list, err := s.store.ListSessions(u.ID, s.now(), current)
	if err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"sessions": list})
}

// RevokeSession signs one of the account's devices out.
func (s *Service) RevokeSession(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !s.sameOrigin(r) {
		fail(w, http.StatusForbidden, "forbidden")
		return
	}
	found, err := s.store.DeleteUserSession(u.ID, chi.URLParam(r, "id"))
	if err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	if !found {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
