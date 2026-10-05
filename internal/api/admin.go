package api

import (
	"context"
	"database/sql"

	"github.com/gazes/gazes/internal/admin"
	"github.com/go-chi/chi/v5"
)

// WithAdmin mounts the admin API (/api/v1/admin). Sessions are identified like the library
// routes (WithLibraryUser override, else the auth session); roles come from the accounts store.
func WithAdmin(svc *admin.Service) Option { return func(s *Server) { s.admin = svc } }

type adminUsers struct{ s *Server }

func (u adminUsers) UserRole(ctx context.Context, id int64) (string, error) {
	if u.s.auth == nil {
		return "", sql.ErrNoRows
	}
	return u.s.auth.UserRole(ctx, id)
}

func (s *Server) mountAdmin(r chi.Router) {
	if s.admin == nil {
		return
	}
	s.admin.SetSessionAuth(s.libraryUserID, adminUsers{s})
	s.wireAdminPlayback()
	s.admin.Mount(r)
}
