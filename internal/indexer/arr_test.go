package indexer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type arrRoundTripper func(*http.Request) (*http.Response, error)

func (f arrRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func arrResponse(r *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Request: r}
}

func TestParseAuthorityMap(t *testing.T) {
	got, err := ParseAuthorityMap(`{"20":7,"101":9}`)
	if err != nil || got[20] != 7 || got[101] != 9 {
		t.Fatalf("map=%v err=%v", got, err)
	}
	for _, raw := range []string{`{"nope":7}`, `{"20":0}`, `{`} {
		if _, err := ParseAuthorityMap(raw); err == nil {
			t.Fatalf("expected %q to fail", raw)
		}
	}
}

func TestArrResolverUsesExplicitSonarrBindingAndVerdicts(t *testing.T) {
	paths := []string{}
	client := &http.Client{Transport: arrRoundTripper(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.RequestURI())
		if r.Header.Get("X-Api-Key") != "secret" {
			t.Fatal("API key missing")
		}
		switch r.URL.Path {
		case "/api/v3/episode":
			if r.URL.Query().Get("seriesId") != "7" || r.URL.Query().Get("seasonNumber") != "4" {
				t.Fatalf("unexpected episode query %s", r.URL.RawQuery)
			}
			return arrResponse(r, `[{"id":91,"seasonNumber":4,"episodeNumber":1}]`), nil
		case "/api/v3/release":
			if r.URL.Query().Get("episodeId") != "91" {
				t.Fatalf("unexpected release query %s", r.URL.RawQuery)
			}
			return arrResponse(r, `[
 {"title":"approved second","downloadUrl":"magnet:?xt=urn:btih:2222222222222222222222222222222222222222","guid":"b","approved":true,"customFormatScore":10,"languages":[{"name":"French"}]},
 {"title":"not approved","downloadUrl":"magnet:?xt=urn:btih:1111111111111111111111111111111111111111","approved":false},
 {"title":"season pack","downloadUrl":"magnet:?xt=urn:btih:3333333333333333333333333333333333333333","approved":true,"fullSeason":true},
 {"title":"rejected","downloadUrl":"magnet:?xt=urn:btih:4444444444444444444444444444444444444444","approved":true,"rejections":["bad"]},
 {"title":"approved first","downloadUrl":"magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","guid":"a","approved":true}
]`), nil
		default:
			t.Fatalf("unexpected title lookup or path %s", r.URL.Path)
			return nil, nil
		}
	})}
	resolver, err := NewArrEpisodeResolver("http://sonarr.test", "secret", `{"123":7}`, "", "", "", client)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolver.ResolvePlaybackSources(context.Background(), EpisodeIdentity{MediaID: 123, Format: "TV", Titles: []string{"Tensura"}, SeasonNumber: 4, EpisodeNumber: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 2 || got.Sources[0].ID != "b" || got.Sources[1].ID != "a" {
		t.Fatalf("authority order/filtering changed: %+v", got.Sources)
	}
	if strings.Contains(strings.Join(paths, " "), "lookup") || got.Sources[0].LanguageTag != LangOther {
		t.Fatalf("local title/language inference leaked into authoritative path: paths=%v source=%+v", paths, got.Sources[0])
	}
}

func TestArrResolverMissingBindingDoesNotCallNetwork(t *testing.T) {
	called := false
	client := &http.Client{Transport: arrRoundTripper(func(r *http.Request) (*http.Response, error) { called = true; return nil, nil })}
	resolver, _ := NewArrEpisodeResolver("http://sonarr.test", "secret", `{}`, "", "", "", client)
	_, err := resolver.ResolveSeasonSources(context.Background(), EpisodeIdentity{MediaID: 999, SeasonNumber: 1, EpisodeNumber: 1})
	if !errors.Is(err, ErrAuthorityMappingMissing) || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestArrResolverDoesNotFallbackWhenAuthorityHasNoRelease(t *testing.T) {
	client := &http.Client{Transport: arrRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v3/episode" {
			return arrResponse(r, `[{"id":9,"seasonNumber":1,"episodeNumber":1}]`), nil
		}
		return arrResponse(r, `[]`), nil
	})}
	resolver, _ := NewArrEpisodeResolver("http://sonarr.test", "secret", `{"1":2}`, "", "", "", client)
	_, err := resolver.ResolvePlaybackSources(context.Background(), EpisodeIdentity{MediaID: 1, SeasonNumber: 1, EpisodeNumber: 1})
	if !errors.Is(err, ErrNoApprovedRelease) {
		t.Fatalf("err=%v", err)
	}
}

func TestArrResolverUsesExplicitRadarrBindingForMovie(t *testing.T) {
	client := &http.Client{Transport: arrRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v3/release" || r.URL.Query().Get("movieId") != "42" || r.Header.Get("X-Api-Key") != "radarr-secret" {
			t.Fatalf("unexpected Radarr request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		return arrResponse(r, `[{"title":"movie","downloadUrl":"magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","guid":"movie-1","approved":true}]`), nil
	})}
	resolver, _ := NewArrEpisodeResolver("", "", "", "http://radarr.test", "radarr-secret", `{"77":42}`, client)
	got, err := resolver.ResolvePlaybackSources(context.Background(), EpisodeIdentity{MediaID: 77, Format: "MOVIE", EpisodeNumber: 1})
	if err != nil || len(got.Sources) != 1 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
