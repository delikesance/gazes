package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gazes/gazes/internal/metadata"
)

// anilistImportCap bounds one import: each title is later resolved to its franchise, which can
// cost AniList calls, so the client never takes more than this at once.
const anilistImportCap = 100

// HandleAniListImport answers GET /api/v1/import/anilist?user=NAME with the titles a public
// AniList list is watching or plans to watch. It costs one upstream request.
func (s *Server) HandleAniListImport(w http.ResponseWriter, r *http.Request) {
	list, err := s.catalogService.AniListUserList(r.Context(), r.URL.Query().Get("user"), anilistImportCap)
	switch {
	case errors.Is(err, metadata.ErrAniListUserNotFound):
		writeImportError(w, http.StatusNotFound, "user_not_found")
		return
	case errors.Is(err, metadata.ErrAniListListPrivate):
		writeImportError(w, http.StatusForbidden, "list_private")
		return
	case err != nil:
		s.catalogFailure(w, r, "failed to import the AniList list", "failed to read the AniList list", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(list)
}

func writeImportError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
