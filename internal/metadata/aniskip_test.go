package metadata

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const aniskipFoundJSON = `{"found":true,"results":[{"interval":{"startTime":2.196,"endTime":122.196},"skipType":"op","skipId":"a","episodeLength":1420.12},{"interval":{"startTime":1325,"endTime":1415},"skipType":"ed","skipId":"b","episodeLength":1420}],"message":"ok","statusCode":200}`

func aniskipService(t *testing.T, h http.HandlerFunc) (*AnimeCatalogService, *httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	s := NewAnimeCatalogService(srv.Client())
	s.aniskipBaseURL = srv.URL
	return s, srv, &hits
}

func ftoa(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func TestSkipTimesFound(t *testing.T) {
	s, _, _ := aniskipService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/skip-times/37430/3" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if got := q["types"]; len(got) != 2 || !(got[0] == "op" && got[1] == "ed" || got[0] == "ed" && got[1] == "op") {
			t.Errorf("types = %v", got)
		}
		if q.Get("episodeLength") != "1420.12" {
			t.Errorf("episodeLength = %q", q.Get("episodeLength"))
		}
		w.Write([]byte(aniskipFoundJSON))
	})
	got, err := s.SkipTimes(t.Context(), 37430, 3, 1420.12)
	if err != nil {
		t.Fatal(err)
	}
	want := []SkipSegment{
		{Kind: SkipKindOpening, Start: 2.196, End: 122.196, Source: "aniskip"},
		{Kind: SkipKindEnding, Start: 1325, End: 1415, Source: "aniskip"},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v", got)
	}
}

func TestSkipTimesNotFound(t *testing.T) {
	s, _, _ := aniskipService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"found":false,"results":[]}`))
	})
	got, err := s.SkipTimes(t.Context(), 1, 1, 1420)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestSkipTimesRejectsOtherEncode(t *testing.T) {
	s, _, _ := aniskipService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.ReplaceAll(aniskipFoundJSON, "1420.12", "1440.065")))
	})
	// the ed result still says 1420 vs 1420.12: only 1440.065 is the other encode
	got, err := s.SkipTimes(t.Context(), 37430, 3, 1420.12)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != SkipKindEnding {
		t.Fatalf("got %+v", got)
	}
}

func TestSkipTimesRejectsBadIntervals(t *testing.T) {
	cases := map[string][2]float64{
		"start>=end":   {100, 100},
		"past end":     {1380, 1422},
		"too short":    {10, 20},
		"too long":     {10, 210},
		"negative":     {-5, 60},
		"exactly 181s": {10, 191},
	}
	for name, iv := range cases {
		t.Run(name, func(t *testing.T) {
			body := `{"found":true,"results":[{"interval":{"startTime":` + ftoa(iv[0]) + `,"endTime":` + ftoa(iv[1]) + `},"skipType":"op","episodeLength":1420}]}`
			s, _, _ := aniskipService(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
			got, err := s.SkipTimes(t.Context(), 1, 1, 1420)
			if err != nil || len(got) != 0 {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}

func TestSkipTimesFirstValidPerKind(t *testing.T) {
	body := `{"found":true,"results":[
	 {"interval":{"startTime":5,"endTime":10},"skipType":"op","episodeLength":1420},
	 {"interval":{"startTime":10,"endTime":100},"skipType":"op","episodeLength":1420},
	 {"interval":{"startTime":20,"endTime":110},"skipType":"op","episodeLength":1420}]}`
	s, _, _ := aniskipService(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
	got, err := s.SkipTimes(t.Context(), 1, 1, 1420)
	if err != nil || len(got) != 1 || got[0].Start != 10 || got[0].End != 100 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestSkipTimesErrors(t *testing.T) {
	old := aniskipTimeout
	aniskipTimeout = 100 * time.Millisecond
	t.Cleanup(func() { aniskipTimeout = old })
	handlers := map[string]http.HandlerFunc{
		"500":     func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		"badjson": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{nope`)) },
		"timeout": func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
		},
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			s, _, _ := aniskipService(t, h)
			if got, err := s.SkipTimes(t.Context(), 1, 1, 1420); err == nil {
				t.Fatalf("expected error, got %+v", got)
			}
		})
	}
}

func TestSkipTimesCaching(t *testing.T) {
	t.Run("found then server gone", func(t *testing.T) {
		s, srv, _ := aniskipService(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(aniskipFoundJSON)) })
		if got, err := s.SkipTimes(t.Context(), 37430, 3, 1420.12); err != nil || len(got) != 2 {
			t.Fatalf("first: %+v, %v", got, err)
		}
		srv.Close()
		if got, err := s.SkipTimes(t.Context(), 37430, 3, 1420.12); err != nil || len(got) != 2 {
			t.Fatalf("second: %+v, %v", got, err)
		}
	})
	t.Run("not found cached", func(t *testing.T) {
		s, _, hits := aniskipService(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			w.Write([]byte(`{"found":false,"results":[]}`))
		})
		for i := 0; i < 2; i++ {
			if got, err := s.SkipTimes(t.Context(), 1, 1, 1420); err != nil || len(got) != 0 {
				t.Fatalf("call %d: %+v, %v", i, got, err)
			}
		}
		if *hits != 1 {
			t.Fatalf("hits = %d", *hits)
		}
	})
	t.Run("error not cached", func(t *testing.T) {
		var n int32
		s, _, hits := aniskipService(t, func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&n, 1) == 1 {
				w.WriteHeader(500)
				return
			}
			w.Write([]byte(aniskipFoundJSON))
		})
		if _, err := s.SkipTimes(t.Context(), 37430, 3, 1420.12); err == nil {
			t.Fatal("expected error")
		}
		if got, err := s.SkipTimes(t.Context(), 37430, 3, 1420.12); err != nil || len(got) != 2 {
			t.Fatalf("retry: %+v, %v", got, err)
		}
		if *hits != 2 {
			t.Fatalf("hits = %d", *hits)
		}
	})
}

func TestSkipTimesInvalidArgs(t *testing.T) {
	s, _, hits := aniskipService(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(aniskipFoundJSON)) })
	for _, a := range []struct {
		mal, ep int
		d       float64
	}{{0, 3, 1420}, {1, 0, 1420}, {1, 3, 0}, {-1, 3, 1420}} {
		got, err := s.SkipTimes(t.Context(), a.mal, a.ep, a.d)
		if err != nil || len(got) != 0 {
			t.Fatalf("%+v: %+v, %v", a, got, err)
		}
	}
	if *hits != 0 {
		t.Fatalf("hits = %d", *hits)
	}
}

func TestFormatCatalogItemMalID(t *testing.T) {
	if got := formatCatalogItem(&aniListMediaItem{ID: 1, IDMal: 37430}, false); got.MalID != 37430 {
		t.Fatalf("MalID = %d", got.MalID)
	}
}
