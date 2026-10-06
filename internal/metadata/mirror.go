package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gazes/gazes/internal/kv"
)

// The mirror copies the whole AniList anime library into the durable store (kv.Durable), one
// page per MirrorStep, so catalog pages answer from disk instead of spending the rate-limited
// AniList budget. Once complete it only reloads, once a day, the entries AniList says changed.
const (
	mirrorPageSize        = 25 // same bound as maxDetailBatch: detailFields is a heavy selection
	mirrorUpdatedPageSize = 50
	mirrorMaxUpdatedPages = 20 // 1000 changed entries a day is far above AniList's real churn
	mirrorRefreshEvery    = 24 * time.Hour
	mirrorRefreshMargin   = time.Hour // overlap between two refreshes: a late AniList write is not missed
	mirrorDetailTTL       = 24 * time.Hour
	mirrorSearchKeep      = 30 * 24 * time.Hour // free-form search results older than this are dropped

	mirrorStateDomain = "mirror"
	mirrorStateKey    = "state"
)

// ErrMirrorDisabled means there is no durable store to mirror into (no Redis or CATALOG_DIR).
var ErrMirrorDisabled = errors.New("catalog mirror disabled: no durable store")

// MirrorProgress says how far the import is.
type MirrorProgress struct {
	Imported    int       `json:"imported"`
	AfterID     int       `json:"after_id"`
	Complete    bool      `json:"complete"`
	LastRefresh time.Time `json:"last_refresh"`
}

type mirrorState struct {
	AfterID     int   `json:"after_id"`
	Imported    int   `json:"imported"`
	Complete    bool  `json:"complete"`
	LastRefresh int64 `json:"last_refresh"` // unix seconds of the last finished import or refresh
}

func (st mirrorState) progress() MirrorProgress {
	return MirrorProgress{Imported: st.Imported, AfterID: st.AfterID, Complete: st.Complete, LastRefresh: time.Unix(st.LastRefresh, 0)}
}

const mirrorPageQuery = `
query ($after: Int, $perPage: Int) {
  Page(perPage: $perPage) {
    pageInfo { hasNextPage }
    media(id_greater: $after, type: ANIME, sort: ID) {` + detailFields + `    }
  }
}
`

const mirrorUpdatedQuery = `
query ($page: Int, $perPage: Int) {
  Page(page: $page, perPage: $perPage) {
    pageInfo { hasNextPage }
    media(type: ANIME, sort: UPDATED_AT_DESC) { id updatedAt }
  }
}
`

type mirrorUpdatedResponse struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Data struct {
		Page struct {
			PageInfo struct {
				HasNextPage bool `json:"hasNextPage"`
			} `json:"pageInfo"`
			Media []struct {
				ID        int   `json:"id"`
				UpdatedAt int64 `json:"updatedAt"`
			} `json:"media"`
		} `json:"Page"`
	} `json:"data"`
}

func (s *AnimeCatalogService) mirrorNow() time.Time {
	if s.mirrorClock != nil {
		return s.mirrorClock()
	}
	return time.Now()
}

func (s *AnimeCatalogService) loadMirror() mirrorState {
	var st mirrorState
	if raw, ok := s.durable.Get(mirrorStateDomain, mirrorStateKey); ok {
		_ = json.Unmarshal(raw, &st)
	}
	return st
}

func (s *AnimeCatalogService) saveMirror(st mirrorState) {
	if raw, err := json.Marshal(st); err == nil {
		s.durable.Set(mirrorStateDomain, mirrorStateKey, raw)
	}
}

// MirrorProgress reports the import without doing any work.
func (s *AnimeCatalogService) MirrorProgress() (MirrorProgress, bool) {
	if s.durable == nil {
		return MirrorProgress{}, false
	}
	s.mirrorMu.Lock()
	defer s.mirrorMu.Unlock()
	return s.loadMirror().progress(), true
}

// MirrorStep does one unit of mirror work: the next page of the import, or, when the import is
// complete and a day old, the refresh of what changed. It returns at once when there is nothing
// to do. A throttle comes back as *RateLimitError so the caller can wait; the cursor only
// advances after a page is stored, so a failed step is simply repeated.
func (s *AnimeCatalogService) MirrorStep(ctx context.Context) (MirrorProgress, error) {
	if s.durable == nil {
		return MirrorProgress{}, ErrMirrorDisabled
	}
	s.mirrorMu.Lock()
	defer s.mirrorMu.Unlock()
	ctx = kv.Background(ctx) // leaves half the burst to visitors
	st := s.loadMirror()
	var err error
	switch {
	case !st.Complete:
		err = s.mirrorImportPage(ctx, &st)
	case s.mirrorNow().Sub(time.Unix(st.LastRefresh, 0)) >= mirrorRefreshEvery:
		err = s.mirrorRefresh(ctx, &st)
	}
	return st.progress(), err
}

func (s *AnimeCatalogService) mirrorImportPage(ctx context.Context, st *mirrorState) error {
	var parsed aniListPageResponse
	if err := s.anilist.post(ctx, mirrorPageQuery, map[string]any{"after": st.AfterID, "perPage": mirrorPageSize}, &parsed); err != nil {
		return err
	}
	if len(parsed.Errors) > 0 {
		return fmt.Errorf("anilist: %s", parsed.Errors[0].Message)
	}
	media := parsed.Data.Page.Media
	for i := range media {
		m := &media[i]
		if m.ID == 0 {
			continue
		}
		item := formatCatalogItem(m, true)
		s.detailC.Put(ctx, fmt.Sprint(m.ID), &item, mirrorDetailTTL)
		st.AfterID = max(st.AfterID, m.ID)
		st.Imported++
	}
	if len(media) == 0 || !parsed.Data.Page.PageInfo.HasNextPage {
		st.Complete = true
		st.LastRefresh = s.mirrorNow().Unix()
	}
	s.saveMirror(*st)
	return nil
}

// mirrorRefresh reloads every entry AniList changed since the last refresh. It first lists the
// recently updated ids (one light request per 50), then fetches them in detail batches.
func (s *AnimeCatalogService) mirrorRefresh(ctx context.Context, st *mirrorState) error {
	since := time.Unix(st.LastRefresh, 0).Add(-mirrorRefreshMargin).Unix()
	var ids []int
scan:
	for page := 1; page <= mirrorMaxUpdatedPages; page++ {
		var parsed mirrorUpdatedResponse
		if err := s.anilist.post(ctx, mirrorUpdatedQuery, map[string]any{"page": page, "perPage": mirrorUpdatedPageSize}, &parsed); err != nil {
			return err
		}
		if len(parsed.Errors) > 0 {
			return fmt.Errorf("anilist: %s", parsed.Errors[0].Message)
		}
		for _, m := range parsed.Data.Page.Media {
			if m.UpdatedAt < since {
				break scan // sorted newest first: everything after is older
			}
			ids = append(ids, m.ID)
		}
		if !parsed.Data.Page.PageInfo.HasNextPage {
			break
		}
	}
	for start := 0; start < len(ids); start += maxDetailBatch {
		batch := ids[start:min(start+maxDetailBatch, len(ids))]
		var parsed aniListPageResponse
		if err := s.anilist.post(ctx, animeDetailBatchQuery, map[string]any{"ids": batch, "perPage": len(batch)}, &parsed); err != nil {
			return err
		}
		if len(parsed.Errors) > 0 {
			return fmt.Errorf("anilist: %s", parsed.Errors[0].Message)
		}
		for i := range parsed.Data.Page.Media {
			m := &parsed.Data.Page.Media[i]
			if m.ID == 0 {
				continue
			}
			item := formatCatalogItem(m, true)
			s.detailC.Put(ctx, fmt.Sprint(m.ID), &item, mirrorDetailTTL)
		}
	}
	s.durable.PruneOlderThan("catalog", mirrorSearchKeep)
	st.LastRefresh = s.mirrorNow().Unix()
	s.saveMirror(*st)
	return nil
}
