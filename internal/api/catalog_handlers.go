package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/metadata"
	"github.com/go-chi/chi/v5"
)

// HandleCatalogTrending handles fetching trending anime from AniList.
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
		diagnostics.Logger(r.Context(), s.logger).Error("failed to get trending anime", "err", err)
		http.Error(w, `{"error": "failed to get trending anime"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
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
		diagnostics.Logger(r.Context(), s.logger).Error("failed to get popular anime", "err", err)
		http.Error(w, `{"error": "failed to get popular anime"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
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

	res, err := s.catalogService.SearchCatalog(r.Context(), q, genre, page, perPage)
	if err != nil {
		diagnostics.Logger(r.Context(), s.logger).Error("failed to search anime catalog", "err", err, "query", q)
		http.Error(w, `{"error": "failed to search catalog"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
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
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(item)
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
		http.Error(w, "failed to load seasons", 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
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
		http.Error(w, "failed to load seasons", 502)
		return nil, nil, nil
	}
	for _, season := range f.Seasons {
		if season.ID == seasonID {
			item, err := s.catalogService.GetAnimeDetailsWithEpisodes(r.Context(), seasonID)
			if err != nil {
				http.Error(w, "failed to load episodes", 502)
				return nil, nil, nil
			}
			return item, f, &season
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
		for _, other := range f.Seasons {
			if other.ID == item.ID {
				continue
			}
			for _, alias := range append(other.Aliases, other.Title) {
				cleanAlias := strings.TrimSpace(alias)
				if cleanAlias == "" {
					continue
				}
				normAlias := indexer.CleanTitleForSearch(cleanAlias)
				if normAlias == "" {
					continue
				}
				isItemAlias := false
				isBaseOfItem := false
				for _, itemAlias := range item.Aliases {
					normItem := indexer.CleanTitleForSearch(itemAlias)
					if normItem == normAlias {
						isItemAlias = true
						break
					}
					if strings.Contains(" "+strings.ToLower(normItem)+" ", " "+strings.ToLower(normAlias)+" ") {
						isBaseOfItem = true
					}
				}
				if !isItemAlias && !isBaseOfItem {
					identity.ExcludedTitles = append(identity.ExcludedTitles, cleanAlias)
				}
				subParts := regexp.MustCompile(`[:：–—\-]`).Split(cleanAlias, -1)
				if len(subParts) > 1 {
					for _, part := range subParts[1:] {
						cleanPart := strings.TrimSpace(part)
						cleanPart = regexp.MustCompile(`(?i)\b(?:season|saison|part|cour)\s*\d+\b`).ReplaceAllString(cleanPart, "")
						cleanPart = strings.TrimSpace(cleanPart)
						if len(cleanPart) >= 3 {
							normPart := strings.ToLower(cleanPart)
							partMatchesItem := false
							for _, itemAlias := range item.Aliases {
								if strings.Contains(strings.ToLower(itemAlias), normPart) {
									partMatchesItem = true
									break
								}
							}
							if !partMatchesItem {
								identity.ExcludedTitles = append(identity.ExcludedTitles, cleanPart)
							}
						}
					}
				}
			}
		}
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
	cacheVersion := "discovery-v3"
	if authoritative {
		cacheVersion = "arr-v1"
	}
	key := fmt.Sprintf("%s|%d|%d", cacheVersion, item.ID, ep)
	if discovery == "full" {
		key += "|full"
	}
	res, hit, err := s.sources().resolve(r.Context(), key, func(ctx context.Context) (*indexer.EpisodeSourcesResponse, error) {
		if discovery == "full" {
			return s.episodeResolver.ResolveSeasonSources(ctx, identity)
		}
		return s.episodeResolver.ResolvePlaybackSources(ctx, identity)
	})
	if err != nil {
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
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = json.NewEncoder(w).Encode(res)
}
