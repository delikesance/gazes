package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gazes/gazes/internal/indexer"
)

type SearchResponse struct {
	Partial bool                  `json:"partial,omitempty"`
	Count   int                   `json:"count"`
	Query   string                `json:"query"`
	Items   []indexer.TorrentItem `json:"items"`
	Groups  []indexer.AnimeGroup  `json:"groups"`
}

// HandleSearch handles torrent search queries against the configured indexer.
func (s *Server) HandleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	cat := r.URL.Query().Get("category")
	sort := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")
	pageStr := r.URL.Query().Get("page")

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	opts := indexer.SearchOptions{
		Query:    q,
		Category: cat,
		SortBy:   sort,
		Order:    order,
		Page:     page,
	}

	items, err := s.indexer.Search(r.Context(), opts)
	var partial *indexer.PartialError
	if err != nil && len(items) == 0 && (!errors.As(err, &partial) || partial.AllFailed) {
		s.logger.Error("indexer search error", "err", err, "query", q)
		http.Error(w, `{"error": "failed to search indexer"}`, http.StatusInternalServerError)
		return
	}

	// Filter out zero-seeder items
	seededItems := make([]indexer.TorrentItem, 0, len(items))
	for _, it := range items {
		if it.Seeders > 0 {
			seededItems = append(seededItems, it)
		}
	}
	items = seededItems

	// Enrich items with anime posters & titles
	items = s.enrichItemsWithAnimeMetadata(r.Context(), items)
	groups := indexer.GroupTorrentsByAnime(items)

	resp := SearchResponse{Partial: err != nil,
		Count:  len(items),
		Query:  q,
		Items:  items,
		Groups: groups,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleLatest handles retrieving latest torrent releases.
func (s *Server) HandleLatest(w http.ResponseWriter, r *http.Request) {
	cat := r.URL.Query().Get("category")
	pageStr := r.URL.Query().Get("page")

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	items, err := s.indexer.GetLatest(r.Context(), cat, page)
	var partial *indexer.PartialError
	if err != nil && len(items) == 0 && (!errors.As(err, &partial) || partial.AllFailed) {
		s.logger.Error("indexer get latest error", "err", err)
		http.Error(w, `{"error": "failed to get latest torrents"}`, http.StatusInternalServerError)
		return
	}

	// Filter out zero-seeder items
	seededItems := make([]indexer.TorrentItem, 0, len(items))
	for _, it := range items {
		if it.Seeders > 0 {
			seededItems = append(seededItems, it)
		}
	}
	items = seededItems

	// Enrich items with anime posters & titles
	items = s.enrichItemsWithAnimeMetadata(r.Context(), items)
	groups := indexer.GroupTorrentsByAnime(items)

	resp := SearchResponse{Partial: err != nil,
		Count:  len(items),
		Query:  "",
		Items:  items,
		Groups: groups,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) enrichItemsWithAnimeMetadata(ctx context.Context, items []indexer.TorrentItem) []indexer.TorrentItem {
	if len(items) == 0 || s.animeService == nil {
		return items
	}

	enriched := make([]indexer.TorrentItem, len(items))
	copy(enriched, items)

	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)

	for i := range enriched {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			meta, err := s.animeService.GetAnimeByTitle(lookupCtx, enriched[idx].Title)
			if err == nil && meta != nil {
				enriched[idx].AnimeDetails = meta
			}
		}(i)
	}

	wg.Wait()
	return enriched
}
