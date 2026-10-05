package admin

import "github.com/go-chi/chi/v5"

// mountViews registers the admin read endpoints of this file's agent (views = overview/views/catalog).
// Only that agent edits this file (and its tests).
func (s *Service) mountViews(r chi.Router) {}
