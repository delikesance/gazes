package admin

import "github.com/go-chi/chi/v5"

// mountPlayback registers the admin read endpoints of this file's agent (playback = playback/costs/issues).
// Only that agent edits this file (and its tests).
func (s *Service) mountPlayback(r chi.Router) {}
