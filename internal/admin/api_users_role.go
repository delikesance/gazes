package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// RoleSetter changes an account's role (implemented by *auth.Service).
type RoleSetter interface {
	SetUserRoleByID(ctx context.Context, id int64, role string) error
}

// SetRoleSetter gives the service write access to account roles (the accounts connection
// of the service itself is read-only). Not safe once requests are being served.
func (s *Service) SetRoleSetter(rs RoleSetter) { s.roles = rs }

func (s *Service) mountUserRole(r chi.Router) {
	// Granting admin is for a human administrator only: a Bearer token can never mint an admin.
	r.With(s.sessionOnly).Post("/users/{id}/role", s.handleUserRole)
}

// handleUserRole: POST /users/{id}/role {role:"admin"}. Promotes an account to administrator.
// It is idempotent. Demotion is deliberately not exposed here (it needs a last-admin guard).
func (s *Service) handleUserRole(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "bad_id", "invalid user id")
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if !issueDecode(w, r, &in) {
		return
	}
	if in.Role != roleAdmin {
		writeAPIError(w, http.StatusBadRequest, "bad_role", `role must be "admin"`)
		return
	}
	if s.roles == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "unavailable", "role management is not available")
		return
	}
	if err := s.roles.SetUserRoleByID(r.Context(), id, roleAdmin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusNotFound, "not_found", "no such user")
			return
		}
		pbServerError(w, err)
		return
	}
	actor, _ := s.actorOf(r)
	s.opsAudit(r.Context(), 0, "user:grant_admin", "by="+actor+" user="+strconv.FormatInt(id, 10), "ok", 0)
	writeDataNoPeriod(w, map[string]any{"user_id": id, "role": roleAdmin})
}
