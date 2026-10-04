package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/library"
	"github.com/gazes/gazes/internal/torrent"
	"github.com/go-chi/chi/v5"
)

const (
	libraryBodyLimit = 4 << 10
	libraryTextLimit = 512
)

// torrentFiles is implemented by engines that can list the files of an already loaded torrent.
type torrentFiles interface {
	Files(infoHash string) ([]torrent.FileInfo, bool)
}

type libraryRequest struct {
	InfoHash    string `json:"info_hash"`
	FileIndex   int    `json:"file_index"`
	ReleaseName string `json:"release_name"`
	AnimeID     int    `json:"anime_id"`
	Title       string `json:"title"`
}

func writeLibraryJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func libraryError(w http.ResponseWriter, status int, msg string) {
	writeLibraryJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) libraryUserID(r *http.Request) (int64, bool) {
	if s.libraryUser != nil {
		return s.libraryUser(r)
	}
	if s.auth == nil {
		return 0, false
	}
	u := s.auth.CurrentUser(r)
	if u == nil {
		return 0, false
	}
	return u.ID, true
}

// sourceMatches reports whether infoHash is one of the sources this server itself resolved for
// (season, ep), with a language that maps to lang (VF and MULTI to "vf", VOSTFR to "vostfr").
// It only reads the findings cached by HandleSeasonSources, stale ones included, and never
// triggers a resolve: a client cannot make the library cache a torrent the server did not offer.
//
// It returns the offered source so that, for a batch (season pack) source, the caller can check that
// the file the client picked really is this episode.
func (s *Server) offeredSource(ctx context.Context, season, ep int, lang, infoHash string) (indexer.EpisodeSource, bool) {
	authoritative := s.cfg != nil && s.cfg.ArrAuthoritative
	cache := s.sources().cache
	for _, full := range []bool{false, true} {
		res, ok := cache.Peek(ctx, sourceCacheKey(authoritative, season, ep, full))
		if !ok || res == nil {
			continue
		}
		for _, src := range res.Sources {
			if !strings.EqualFold(src.InfoHash, infoHash) {
				continue
			}
			switch src.LanguageTag {
			case indexer.LangVF, indexer.LangMULTI:
				if lang == "vf" {
					return src, true
				}
			case indexer.LangVOSTFR:
				if lang == "vostfr" {
					return src, true
				}
			}
		}
	}
	return indexer.EpisodeSource{}, false
}

// batchFileMatches reports whether file is the episode the offered pack source was resolved for, using the
// same episode matcher as the resolver on the file's base name. Anything but a definite match (an
// unnumbered file, another episode, an extra) is refused.
func batchFileMatches(src indexer.EpisodeSource, file string) bool {
	identity := indexer.EpisodeIdentity{
		Titles:          src.AnimeAliases,
		ExcludedTitles:  src.ExcludedTitles,
		SeasonNumber:    max(1, src.SeasonNumber),
		EpisodeNumber:   src.EpisodeNumber,
		TaggedEpisode:   src.TaggedEpisode,
		AbsoluteEpisode: src.AbsoluteEpisode,
	}
	identity.AllowUnqualified = identity.SeasonNumber <= 1 // as HandleSeasonSources: later seasons need a qualified name
	if len(identity.Titles) == 0 && src.AnimeTitle != "" {
		identity.Titles = []string{src.AnimeTitle}
	}
	match, batch, _ := indexer.NewEpisodeMatcher(identity).Match(path.Base(strings.ReplaceAll(file, "\\", "/")))
	return match && !batch
}

func routeInt(r *http.Request, name string) (int, bool) {
	n, err := strconv.Atoi(chi.URLParam(r, name))
	return n, err == nil && n > 0
}

// HandleLibraryRegister asks the library to keep a copy of one episode file.
func (s *Server) HandleLibraryRegister(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.libraryUserID(r)
	if !ok {
		libraryError(w, http.StatusUnauthorized, "login required")
		return
	}
	season, okS := routeInt(r, "season")
	ep, okE := routeInt(r, "ep")
	lang := chi.URLParam(r, "lang")
	if !okS || !okE || (lang != "vostfr" && lang != "vf") {
		libraryError(w, http.StatusUnprocessableEntity, "invalid season, episode or language")
		return
	}
	var body libraryRequest
	r.Body = http.MaxBytesReader(w, r.Body, libraryBodyLimit)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		libraryError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.InfoHash = strings.ToLower(body.InfoHash)
	if _, err := hex.DecodeString(body.InfoHash); err != nil || len(body.InfoHash) != 40 || body.FileIndex < 0 ||
		body.AnimeID <= 0 || len(body.ReleaseName) > libraryTextLimit || len(body.Title) > libraryTextLimit {
		libraryError(w, http.StatusUnprocessableEntity, "invalid torrent reference")
		return
	}

	if s.catalogService == nil {
		libraryError(w, http.StatusServiceUnavailable, "catalog unavailable")
		return
	}
	item, err := s.catalogService.GetAnimeDetailsWithEpisodes(r.Context(), season)
	if err != nil {
		diagnostics.Logger(r.Context(), s.logger).Warn("library.catalog_failed", "err", err, "season_id", season)
		libraryError(w, http.StatusServiceUnavailable, "catalog unavailable")
		return
	}
	found := false
	for _, e := range item.EpisodeList {
		if e.EpisodeNumber == ep && !e.Upcoming {
			found = true
			break
		}
	}
	if !found {
		libraryError(w, http.StatusUnprocessableEntity, "episode not in season")
		return
	}

	src, offered := s.offeredSource(r.Context(), season, ep, lang, body.InfoHash)
	if !offered {
		libraryError(w, http.StatusUnprocessableEntity, "source_not_resolved")
		return
	}

	lister, ok := s.torrentEngine.(torrentFiles)
	if !ok {
		libraryError(w, http.StatusUnprocessableEntity, "torrent not loaded")
		return
	}
	files, loaded := lister.Files(body.InfoHash)
	if !loaded {
		libraryError(w, http.StatusUnprocessableEntity, "torrent not loaded")
		return
	}
	if body.FileIndex >= len(files) || !files[body.FileIndex].IsVideo {
		libraryError(w, http.StatusUnprocessableEntity, "file is not a video")
		return
	}
	// In a season pack the client chooses the file: it must be this episode, not another one of the pack.
	if src.IsBatch && !batchFileMatches(src, files[body.FileIndex].Path) {
		libraryError(w, http.StatusUnprocessableEntity, "file_not_episode")
		return
	}

	entry, created, err := s.library.Register(userID, library.Request{
		Key:         library.Key{SeasonID: season, Episode: ep, Lang: lang},
		AnimeID:     body.AnimeID,
		Title:       body.Title,
		InfoHash:    body.InfoHash,
		FileIndex:   body.FileIndex,
		ReleaseName: body.ReleaseName,
	})
	switch {
	case errors.Is(err, library.ErrRateLimited), errors.Is(err, library.ErrBusy):
		libraryError(w, http.StatusTooManyRequests, "too many cached episodes, try again later")
		return
	case errors.Is(err, library.ErrNoSpace):
		libraryError(w, http.StatusInsufficientStorage, "no space left in the library")
		return
	case err != nil:
		diagnostics.Logger(r.Context(), s.logger).Error("library.register_failed", "err", err, "season_id", season, "episode", ep, "lang", lang)
		libraryError(w, http.StatusInternalServerError, "library error")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeLibraryJSON(w, status, map[string]any{"created": created, "state": string(entry.State)})
}

// HandleLibraryCopies lists the ready library copies of an episode.
func (s *Server) HandleLibraryCopies(w http.ResponseWriter, r *http.Request) {
	season, okS := routeInt(r, "season")
	ep, okE := routeInt(r, "ep")
	if !okS || !okE {
		libraryError(w, http.StatusBadRequest, "invalid season or episode")
		return
	}
	copies, err := s.library.Copies(season, ep)
	if err != nil {
		diagnostics.Logger(r.Context(), s.logger).Error("library.list_failed", "err", err)
		libraryError(w, http.StatusInternalServerError, "library error")
		return
	}
	if copies == nil {
		copies = []library.Copy{}
	}
	writeLibraryJSON(w, http.StatusOK, map[string]any{"copies": copies})
}
