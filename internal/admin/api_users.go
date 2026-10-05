package admin

import "github.com/go-chi/chi/v5"

// mountUsers registers the admin read endpoints of this file's agent (users = users/growth).
// Only that agent edits this file (and its tests).
func (s *Service) mountUsers(r chi.Router) {}
