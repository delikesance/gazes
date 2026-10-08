package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/auth"
	"github.com/gazes/gazes/internal/config"
	"github.com/gazes/gazes/internal/indexer"
)

type failingProvider struct{}

func (failingProvider) Name() string { return "tracker" }
func (failingProvider) Search(context.Context, indexer.SearchOptions) ([]indexer.TorrentItem, error) {
	return nil, errors.New("down")
}
func (failingProvider) GetLatest(context.Context, string, int) ([]indexer.TorrentItem, error) {
	return nil, errors.New("down")
}

type opsServer struct {
	s       *Server
	adminDB *sql.DB
	token   string
}

func newOpsServer(t *testing.T, idx indexer.Provider) opsServer {
	t.Helper()
	dir := t.TempDir()
	as, err := auth.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { as.Close() })
	st, err := admin.Open(filepath.Join(dir, "admin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc, err := admin.NewService(st, filepath.Join(dir, "accounts.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	tok, _, err := st.CreateToken(context.Background(), "t", []string{admin.ScopeOpsWrite, admin.ScopeConfigWrite}, 0)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(dir, "admin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewServer(&config.Config{}, nil, idx, nil, nil, WithAdmin(svc), WithLibraryUser(func(*http.Request) (int64, bool) { return 0, false }))
	return opsServer{s: s, adminDB: db, token: tok}
}

func (o opsServer) action(t *testing.T, name, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/admin/ops/actions/"+name, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+o.token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	o.s.Router().ServeHTTP(rec, req)
	var env struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env.Data
}

func TestAdminOpsReachTheIndexer(t *testing.T) {
	mp := indexer.NewMultiProvider(failingProvider{})
	o := newOpsServer(t, mp)
	ctx := context.Background()
	for i := 0; i < 5; i++ { // repeated failures open the breaker
		_, _ = mp.Search(ctx, indexer.SearchOptions{Query: "q" + string(rune('a'+i))})
	}
	if h, _ := mp.Health(ctx, "tracker"); h.Cooldown == 0 {
		t.Fatalf("breaker did not open: %+v", h)
	}
	if code, data := o.action(t, "retry_source", `{"args":{"source":"tracker"},"dry_run":false}`); code != 200 || data["status"] != "executed" {
		t.Fatalf("retry_source = %d %v", code, data)
	}
	if h, _ := mp.Health(ctx, "tracker"); h.Cooldown != 0 {
		t.Fatalf("breaker still open: %+v", h)
	}
	// a sensitive action through the real server only files an approval
	if code, data := o.action(t, "pause_source", `{"args":{"source":"tracker","minutes":30},"dry_run":false,"justification":"j","expected_effect":"e"}`); code != 409 {
		t.Fatalf("pausing the only source = %d %v", code, data)
	}
}

func TestAdminOpsSourceCache(t *testing.T) {
	o := newOpsServer(t, indexer.NewMultiProvider(failingProvider{}))
	ctx := context.Background()
	o.s.sources().cache.Put(ctx, sourceCacheKey(false, 10, 2, false), &indexer.EpisodeSourcesResponse{Sources: []indexer.EpisodeSource{{TorrentItem: indexer.TorrentItem{InfoHash: "a"}}, {TorrentItem: indexer.TorrentItem{InfoHash: "b"}}}}, time.Minute)
	if n, ok := o.s.cachedEpisodeSources(ctx, 10, 2); !ok || n != 2 {
		t.Fatalf("cached = %d %v", n, ok)
	}
	if code, data := o.action(t, "warm_cache", `{"args":{"season_id":10,"episode":2},"dry_run":false}`); code != 200 || data["result"].(map[string]any)["started"] != false {
		t.Fatalf("warm a cached episode = %d %v", code, data)
	}
	if _, err := o.s.purgeCache(ctx, "episode_sources"); err != nil {
		t.Fatal(err)
	}
	if _, ok := o.s.cachedEpisodeSources(ctx, 10, 2); ok {
		t.Fatal("purge left the episode cached")
	}
	if _, err := o.s.purgeCache(ctx, "catalog"); err == nil {
		t.Fatal("unknown scope accepted")
	}
	// the warm slots bound the background resolutions
	for i := 0; i < opsWarmSlots; i++ {
		o.s.opsWarm <- struct{}{}
	}
	if o.s.warmEpisode(10, 3, false) {
		t.Fatal("a warm started with every slot taken")
	}
}

func TestStreamLimitReached(t *testing.T) {
	o := newOpsServer(t, nil)
	ctx := context.Background()
	active := func(n int) func() int { return func() int { return n } }
	if _, full := o.s.streamLimitReached(ctx, active(500)); full {
		t.Fatal("no limit set, yet refused")
	}
	if _, err := o.adminDB.Exec(`INSERT INTO settings(key, value, updated_at, updated_by) VALUES ('ops.max_concurrent_streams', '3', 0, 't')`); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		active int
		full   bool
	}{{2, false}, {3, true}, {4, true}} {
		if limit, full := o.s.streamLimitReached(ctx, active(c.active)); limit != 3 || full != c.full {
			t.Errorf("%d active: limit %d full %v", c.active, limit, full)
		}
	}
	noAdmin := NewServer(&config.Config{}, nil, nil, nil, nil)
	if _, full := noAdmin.streamLimitReached(ctx, active(500)); full {
		t.Fatal("without the admin panel there is no limit")
	}
}
