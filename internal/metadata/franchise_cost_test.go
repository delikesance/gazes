package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// franchiseUpstream serves a hub show: season 1 (id 1) with a chain of main seasons 2..4 and
// `extras` side stories, answering single lookups and id_in batches like AniList does.
func franchiseUpstream(calls *atomic.Int32, extras int) *http.Client {
	media := func(id int) string {
		var edges []string
		switch {
		case id == 1:
			edges = append(edges, `{"relationType":"SEQUEL","node":{"id":2,"format":"TV"}}`)
			for e := 0; e < extras; e++ {
				edges = append(edges, fmt.Sprintf(`{"relationType":"SIDE_STORY","node":{"id":%d,"format":"OVA"}}`, 100+e))
			}
		case id >= 2 && id <= 4:
			edges = append(edges, fmt.Sprintf(`{"relationType":"PREQUEL","node":{"id":%d,"format":"TV"}}`, id-1))
			if id < 4 {
				edges = append(edges, fmt.Sprintf(`{"relationType":"SEQUEL","node":{"id":%d,"format":"TV"}}`, id+1))
			}
		}
		format := "TV"
		if id >= 100 {
			format = "OVA"
		}
		return fmt.Sprintf(`{"id":%d,"format":"%s","title":{"romaji":"Entry %d"},"startDate":{"year":%d,"month":1,"day":1},"episodes":12,"relations":{"edges":[%s]}}`, id, format, id, 2000+id, strings.Join(edges, ","))
	}
	return &http.Client{Transport: discoveryTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		var body struct {
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var answer string
		if ids, ok := body.Variables["ids"].([]any); ok {
			var all []string
			for _, id := range ids {
				all = append(all, media(int(id.(float64))))
			}
			answer = `{"data":{"Page":{"media":[` + strings.Join(all, ",") + `]}}}`
		} else {
			answer = `{"data":{"Media":` + media(int(body.Variables["id"].(float64))) + `}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(answer)), Header: make(http.Header)}, nil
	})}
}

func TestFranchiseWalkBatchesItsLookups(t *testing.T) {
	var calls atomic.Int32
	service := NewAnimeCatalogService(franchiseUpstream(&calls, 20))
	f, err := service.GetFranchise(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Complete || len(f.Seasons) != 4+20 {
		t.Fatalf("complete=%v seasons=%d", f.Complete, len(f.Seasons))
	}
	// 1 root lookup + one batch per wave: season 2 and 4 first, then season 1 with its extras…
	if got := calls.Load(); got > 6 {
		t.Fatalf("walk spent %d AniList calls for 24 entries, want at most 6", got)
	}
}

func TestFranchiseWalkBatchesNeverExceedTheLimit(t *testing.T) {
	var calls atomic.Int32
	service := NewAnimeCatalogService(franchiseUpstream(&calls, 70))
	f, err := service.GetFranchise(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Seasons) < 70 {
		t.Fatalf("seasons=%d complete=%v", len(f.Seasons), f.Complete)
	}
	if got := calls.Load(); got > 8 {
		t.Fatalf("walk spent %d AniList calls", got)
	}
}
