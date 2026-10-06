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

// HandleMALImport answers POST /api/v1/import/mal {"ids":[...]}: MyAnimeList ids (from the export
// file the browser parsed) mapped to AniList titles, 50 ids per upstream request, at most 150 ids.
func (s *Server) HandleMALImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int `json:"ids"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body) != nil || len(body.IDs) > 5000 {
		writeImportError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	list, err := s.catalogService.AniListByMalIDs(r.Context(), body.IDs, malImportCap)
	if err != nil {
		s.catalogFailure(w, r, "failed to map MyAnimeList ids", "failed to read the MyAnimeList list", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(list)
}

// malImportCap is three AniList requests; like the AniList import, each title then costs a franchise lookup.
const malImportCap = 150
