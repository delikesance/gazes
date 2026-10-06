package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"io"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/metadata"
	"github.com/go-chi/chi/v5"
)

// HandleCatalogTrending handles fetching trending anime from AniList.
// catalogCacheControl lets browsers and the Next.js server reuse a catalog answer for five minutes
// and show it instantly for a day while a fresh copy loads: the data behind it is kept for good.
const catalogCacheControl = "public, max-age=300, stale-while-revalidate=86400"

func (s *Server) HandleCatalogTrending(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	perPageStr := r.URL.Query().Get("per_page")

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	perPage := 20
	if perPageStr != "" {
		if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 {
			perPage = pp
		}
	}

	res, err := s.catalogService.GetTrending(r.Context(), page, perPage)
	if err != nil {
		s.catalogFailure(w, r, "failed to get trending anime", "failed to get trending anime", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", catalogCacheControl)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

// HandleCatalogPopular handles fetching all-time popular anime.
func (s *Server) HandleCatalogPopular(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	perPageStr := r.URL.Query().Get("per_page")

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	perPage := 20
	if perPageStr != "" {
		if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 {
			perPage = pp
		}
	}

	res, err := s.catalogService.GetPopular(r.Context(), page, perPage)
	if err != nil {
		s.catalogFailure(w, r, "failed to get popular anime", "failed to get popular anime", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", catalogCacheControl)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

// HandleCatalogForYou serves the "suggestions" feed. The taste profile comes from the signed-in
// viewer's stored watch log, or from the sessions a visitor posts ({"sessions":[...]}, their
// local log). Without any, ?ids=1,2,3 (most recent first) seeds plain recommendations.
func (s *Server) HandleCatalogForYou(w http.ResponseWriter, r *http.Request) {
	page, perPage := 1, 24
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	if pp, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && pp > 0 {
		perPage = pp
	}
	seeds := []int{}
	for _, raw := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if id, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && id > 0 {
			seeds = append(seeds, id)
		}
	}

	var sessions []metadata.TasteSession
	if s.auth != nil {
		if stored, ok := s.auth.SessionsFor(r, 500); ok {
			for _, w := range stored {
				sessions = append(sessions, metadata.TasteSession{AnimeID: int(w.AnimeID), SeasonID: int(w.SeasonID), Genres: w.Genres, WatchedSeconds: w.WatchedSeconds, Duration: w.Duration, Completed: w.Completed, UpdatedAt: w.UpdatedAt})
			}
		}
	}
	var hidden []int
	if s.auth != nil {
		for _, id := range s.auth.HiddenFor(r) {
			hidden = append(hidden, int(id))
		}
	}
	if r.Method == http.MethodPost {
		var body struct {
			Hidden   []int `json:"hidden"`
			Sessions []struct {
				AnimeID        int      `json:"anime_id"`
				SeasonID       int      `json:"season_id"`
				Genres         []string `json:"genres"`
				WatchedSeconds float64  `json:"watched_seconds"`
				Duration       float64  `json:"duration"`
				Completed      bool     `json:"completed"`
				UpdatedAt      int64    `json:"updated_at"`
			} `json:"sessions"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 256<<10)).Decode(&body) != nil || len(body.Sessions) > 500 || len(body.Hidden) > 2000 {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		if len(hidden) == 0 {
			hidden = body.Hidden
		}
		if len(sessions) > 0 {
			body.Sessions = nil
		}
		for _, v := range body.Sessions {
			if len(v.Genres) > 20 {
				v.Genres = v.Genres[:20]
			}
			sessions = append(sessions, metadata.TasteSession{AnimeID: v.AnimeID, SeasonID: v.SeasonID, Genres: v.Genres, WatchedSeconds: v.WatchedSeconds, Duration: v.Duration, Completed: v.Completed, UpdatedAt: v.UpdatedAt})
		}
	}
	taste := metadata.BuildTaste(sessions, time.Now())
	for _, id := range hidden {
		if id > 0 {
			taste.Dropped[id] = true
		}
	}
	if len(taste.Seeds) > 0 {
		seeds = taste.Seeds
	}

	res, err := s.catalogService.GetForYou(r.Context(), seeds, taste, page, perPage)
	if err != nil {
		s.catalogFailure(w, r, "failed to get suggestions", "failed to get suggestions", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

func splitList(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// HandleCatalogSearch handles searching the anime catalog by title and/or genre.
func (s *Server) HandleCatalogSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	genre := r.URL.Query().Get("genre")
	pageStr := r.URL.Query().Get("page")
	perPageStr := r.URL.Query().Get("per_page")

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	perPage := 24
	if perPageStr != "" {
		if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 {
			perPage = pp
		}
	}

	// genres / exclude are comma separated; the single "genre" parameter stays supported.
	include := splitList(r.URL.Query().Get("genres"))
	if genre != "" {
		include = append(include, genre)
	}
	exclude := splitList(r.URL.Query().Get("exclude"))

	res, err := s.catalogService.SearchCatalogFiltered(r.Context(), q, include, exclude, page, perPage)
	if err != nil {
		s.catalogFailure(w, r, "failed to search anime catalog", "failed to search catalog", err, "query", q)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", catalogCacheControl)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

// HandleCatalogAnimeDetail handles retrieving full anime details and episode list by AniList ID.
func (s *Server) HandleCatalogAnimeDetail(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.Error(w, `{"error": "invalid anime id"}`, http.StatusBadRequest)
		return
	}

	item, err := s.catalogService.GetAnimeDetailsWithEpisodes(r.Context(), id)
	if err != nil {
		diagnostics.Logger(r.Context(), s.logger).Error("failed to get anime details with episodes", "err", err, "id", id)
		http.Error(w, `{"error": "failed to get anime details"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", catalogCacheControl)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(item.InSeason(0))
}

// HandleEpisodeSources resolves all torrent swarms for a specific anime episode, prioritized for French (VOSTFR/VF).
func (s *Server) HandleEpisodeSources(w http.ResponseWriter, r *http.Request) {
	r = sourceContext(w, r)
	// Legacy URLs identify the selected catalog media, not the canonical franchise.
	route := chi.RouteContext(r.Context())
	route.URLParams.Add("season", chi.URLParam(r, "id"))
	s.HandleSeasonSources(w, r)
}

// HandleFranchise returns the complete continuity and separately grouped extras.
func (s *Server) HandleFranchise(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid anime id", 400)
		return
	}
	f, err := s.catalogService.GetFranchise(r.Context(), id)
	if err != nil {
		s.catalogFailure(w, r, "failed to load franchise", "failed to load seasons", err, "anime_id", id)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", catalogCacheControl)
	_ = json.NewEncoder(w).Encode(f)
}

func (s *Server) seasonContext(w http.ResponseWriter, r *http.Request) (*metadata.AnimeCatalogItem, *metadata.Franchise, *metadata.AnimeSeason) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	seasonID, e := strconv.Atoi(chi.URLParam(r, "season"))
	if err != nil || e != nil || id <= 0 || seasonID <= 0 {
		http.Error(w, "invalid anime or season id", 400)
		return nil, nil, nil
	}
	f, err := s.catalogService.GetFranchise(r.Context(), id)
	if err != nil {
		s.catalogFailure(w, r, "failed to load franchise", "failed to load seasons", err, "anime_id", id)
		return nil, nil, nil
	}
	for _, season := range f.Seasons {
		if season.ID == seasonID {
			item, err := s.catalogService.GetAnimeDetailsWithEpisodes(r.Context(), seasonID)
			if err != nil {
				s.catalogFailure(w, r, "failed to load episodes", "failed to load episodes", err, "anime_id", id, "season_id", seasonID)
				return nil, nil, nil
			}
			placed := item.InSeason(season.EpisodeOffset)
			return &placed, f, &season
		}
	}
	if !f.Complete {
		http.Error(w, "season membership unavailable; retry", 503)
	} else {
		http.Error(w, "season does not belong to anime", 404)
	}
	return nil, nil, nil
}

func (s *Server) HandleSeason(w http.ResponseWriter, r *http.Request) {
	item, _, _ := s.seasonContext(w, r)
	if item == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", catalogCacheControl)
	_ = json.NewEncoder(w).Encode(item)
}

func (s *Server) HandleSeasonSources(w http.ResponseWriter, r *http.Request) {
	discovery := r.URL.Query().Get("discovery")
	if discovery != "" && discovery != "fast" && discovery != "full" {
		http.Error(w, "invalid discovery mode", http.StatusBadRequest)
		return
	}
	r = sourceContext(w, r)
	item, f, season := s.seasonContext(w, r)
	if item == nil {
		return
	}
	ep, err := strconv.Atoi(chi.URLParam(r, "ep"))
	if err != nil || ep <= 0 {
		http.Error(w, "invalid episode number", 400)
		return
	}
	exists := false
	for _, entry := range item.EpisodeList {
		if entry.EpisodeNumber == ep {
			exists = true
			if entry.Upcoming {
				http.Error(w, "episode has not aired", 409)
				return
			}
		}
	}
	if !exists {
		http.Error(w, "episode not listed for season", 404)
		return
	}
	isMovie := item.Format == "MOVIE"
	isOVA := item.Format == "OVA" || item.Format == "SPECIAL"
	identity := indexer.EpisodeIdentity{
		MediaID:          item.ID,
		Format:           item.Format,
		Titles:           item.Aliases,
		SeasonNumber:     max(1, season.ReleaseSeasonNumber),
		EpisodeNumber:    ep,
		PartNumber:       indexer.ExtractPartNumber(item.DisplayTitle),
		Standalone:       isMovie,
		IsOVA:            isOVA,
		AllowUnqualified: season.SeasonNumber <= 1,
	}
	authoritative := s.cfg != nil && s.cfg.ArrAuthoritative
	if !authoritative && season.SeasonNumber > 1 {
		canonical, err := s.catalogService.GetAnimeDetailsWithEpisodes(r.Context(), f.ID)
		if err == nil {
			identity.UnqualifiedTitles = indexer.UniqueSeasonAliases(item.Aliases, canonical.Aliases)
			if identity.SeasonNumber == 1 && len(identity.UnqualifiedTitles) > 0 {
				identity.Titles = identity.UnqualifiedTitles
				identity.AllowUnqualified = true
			}
		}
	}

	// Legacy-only title heuristics. In authoritative mode, *Arr has already made
	// these semantic decisions and Gazes never rewrites or reinterprets titles.
	if !authoritative {
		identity.ExcludedTitles = buildExcludedTitles(item, f.Seasons)
	}

	if override, ok := indexer.NumberingOverrides[item.ID]; ok && !authoritative {
		if len(override.Titles) > 0 {
			identity.Titles = override.Titles
			identity.AllowUnqualified = true
		}
		if override.TaggedOffset > 0 {
			identity.TaggedEpisode = ep + override.TaggedOffset
		}
		if override.Season > 0 {
			identity.SeasonNumber = override.Season
		}
		if override.Part > 0 {
			identity.PartNumber = override.Part
		}
		if override.AbsoluteOffset > 0 {
			identity.AbsoluteEpisode = ep + override.AbsoluteOffset
		}
	}

	if s.logger != nil {
		diagnostics.Logger(r.Context(), s.logger).Info("resolving season sources", "anime", item.DisplayTitle, "season_id", item.ID, "season_num", identity.SeasonNumber, "episode", ep, "excluded", len(identity.ExcludedTitles))
	}

	// Do not reuse findings produced by the old RSS discovery/title rejection rules.
	key := sourceCacheKey(authoritative, item.ID, ep, discovery == "full")
	res, hit, err := s.sources().resolve(r.Context(), key, func(ctx context.Context) (*indexer.EpisodeSourcesResponse, error) {
		if discovery == "full" {
			return s.episodeResolver.ResolveSeasonSources(ctx, identity)
		}
		return s.episodeResolver.ResolvePlaybackSources(ctx, identity)
	})
	if err != nil {
		s.recordSourceError(r.Context(), item.ID, ep, err)
		sourceFailure(w, r, err)
		return
	}
	// Cached findings are shared; each response carries the caller's own ids.
	reply := *res
	reply.RequestID = diagnostics.Get(r.Context()).RequestID
	reply.PlaybackSessionID = diagnostics.Get(r.Context()).SessionID
	if hit && s.logger != nil {
		diagnostics.Logger(r.Context(), s.logger).Info("sources.cache_hit", "season_id", item.ID, "episode", ep, "sources", len(res.Sources))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(&reply)
}

// HandleCatalogSeasonal returns releases belonging to the current calendar season.
func (s *Server) HandleCatalogSeasonal(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	result, err := s.catalogService.GetCurrentSeason(r.Context(), page, perPage)
	if err != nil {
		diagnostics.Logger(r.Context(), s.logger).Error("failed to get seasonal anime", "err", err)
		http.Error(w, "failed to get seasonal anime", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", catalogCacheControl)
	_ = json.NewEncoder(w).Encode(result)
}

// HandleCatalogSchedule returns the episodes airing between two unix timestamps.
func (s *Server) HandleCatalogSchedule(w http.ResponseWriter, r *http.Request) {
	from, err1 := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, err2 := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if err1 != nil || err2 != nil || to <= from || to-from > 42*24*3600 {
		http.Error(w, `{"error": "from and to are required unix timestamps spanning at most 42 days"}`, http.StatusBadRequest)
		return
	}
	res, err := s.catalogService.GetSchedule(r.Context(), from, to)
	if err != nil {
		diagnostics.Logger(r.Context(), s.logger).Error("failed to get release schedule", "err", err)
		var limited *metadata.RateLimitError
		if errors.As(err, &limited) {
			// AniList is throttling: tell the client when to come back instead of a bare 502.
			w.Header().Set("Retry-After", strconv.Itoa(int(limited.RetryAfter.Seconds())+1))
			http.Error(w, `{"error": "release schedule temporarily rate limited"}`, http.StatusServiceUnavailable)
			return
		}
		http.Error(w, `{"error": "failed to get release schedule"}`, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", catalogCacheControl)
	_ = json.NewEncoder(w).Encode(res)
}

// catalogFailure answers a failed catalog call. An upstream throttle is a 503 with Retry-After,
// not a 500; every failure is logged with its error chain and goroutine stack, and the response
// carries the request id so the on-screen error can be matched with that log line.
func (s *Server) catalogFailure(w http.ResponseWriter, r *http.Request, event, message string, err error, attrs ...any) {
	status := http.StatusInternalServerError
	var limited *metadata.RateLimitError
	if errors.As(err, &limited) {
		status = http.StatusServiceUnavailable
		w.Header().Set("Retry-After", strconv.Itoa(int(limited.RetryAfter.Seconds())+1))
	}
	requestID := diagnostics.Get(r.Context()).RequestID
	fields := append([]any{"err", err, "error_type", fmt.Sprintf("%T", err), "status", status, "path", r.URL.Path, "query_string", r.URL.RawQuery}, attrs...)
	if status == http.StatusInternalServerError {
		fields = append(fields, "stack", string(debug.Stack()))
	}
	diagnostics.Logger(r.Context(), s.logger).Error(event, fields...)
	w.Header().Set("Content-Type", "application/json")
	if requestID != "" {
		w.Header().Set("X-Request-ID", requestID)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message, "request_id": requestID})
}

var (
	exclusionSplitRegex  = regexp.MustCompile(`[:：–—\-]`)
	seasonMarkerKeyRegex = regexp.MustCompile(`(?i)\b(?:\d+(?:st|nd|rd|th)\s*(?:season|saison|part|partie|cour)|(?:season|saison|part|partie|cour)\s*\d+(?:st|nd|rd|th)?|s\d+)\b`)
	nonAlnumKeyRegex     = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

// compactTitleKey reduces a title to lowercase letters and digits with every
// season marker removed, so "ReZero", "Re:Zero" and "Re:Zero 2nd Season" agree.
func compactTitleKey(title string) string {
	t := seasonMarkerKeyRegex.ReplaceAllString(title, " ")
	t = indexer.CleanTitleForSearch(t)
	return nonAlnumKeyRegex.ReplaceAllString(strings.ToLower(t), "")
}

// isMainSeries reports whether a franchise entry is a numbered main-series
// season. It reuses the franchise grouping ("main") but only for Format TV:
// the grouping also files TV_SHORT/ONA spin-offs (Break Time, Re:PETIT) under
// "main", and those have their own releases that must stay excluded.
func isMainSeries(season metadata.AnimeSeason) bool {
	return season.Group == "main" && season.Format == "TV"
}

// buildExcludedTitles lists titles of the franchise's other entries that must
// reject a release. Other main-series seasons differ from the requested one
// only by season number, which the resolver's season check enforces, so their
// titles (in any language) are never exclusions. A candidate that reduces to something contained in one of
// the item's own titles is the shared franchise base: excluding it would reject
// the item's own releases, and telling seasons apart is the season check's job.
func buildExcludedTitles(item *metadata.AnimeCatalogItem, seasons []metadata.AnimeSeason) []string {
	own := append([]string{item.DisplayTitle, item.TitleEnglish, item.TitleRomaji}, item.Aliases...)
	var ownKeys []string
	for _, t := range own {
		if k := compactTitleKey(t); k != "" {
			ownKeys = append(ownKeys, k)
		}
	}
	collides := func(candidate string) bool {
		key := compactTitleKey(candidate)
		if len(key) < 3 {
			return true
		}
		for _, k := range ownKeys {
			if strings.Contains(k, key) {
				return true
			}
		}
		return false
	}
	var excluded []string
	seen := map[string]bool{}
	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			excluded = append(excluded, t)
		}
	}
	for _, other := range seasons {
		if other.ID == item.ID || isMainSeries(other) {
			continue
		}
		for _, alias := range append(append([]string{}, other.Aliases...), other.Title) {
			cleanAlias := strings.TrimSpace(alias)
			normAlias := indexer.CleanTitleForSearch(cleanAlias)
			if normAlias == "" {
				continue
			}
			isItemAlias := false
			for _, itemAlias := range item.Aliases {
				if indexer.CleanTitleForSearch(itemAlias) == normAlias {
					isItemAlias = true
					break
				}
			}
			if !isItemAlias && !collides(cleanAlias) {
				add(cleanAlias)
			}
			if parts := exclusionSplitRegex.Split(cleanAlias, -1); len(parts) > 1 {
				for _, part := range parts[1:] {
					cleanPart := strings.TrimSpace(seasonMarkerKeyRegex.ReplaceAllString(part, ""))
					if len(cleanPart) >= 3 && !collides(cleanPart) {
						add(cleanPart)
					}
				}
			}
		}
	}
	return excluded
}

// sourceCacheKey names the cached findings of one episode. Findings produced by the old RSS
// discovery/title rejection rules are never reused: the version prefix changes with the rules.
func sourceCacheKey(authoritative bool, seasonID, ep int, full bool) string {
	version := "discovery-v3"
	if authoritative {
		version = "arr-v1"
	}
	key := fmt.Sprintf("%s|%d|%d", version, seasonID, ep)
	if full {
		key += "|full"
	}
	return key
}
