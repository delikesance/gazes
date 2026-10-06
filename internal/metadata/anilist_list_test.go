package metadata

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type listTransport struct {
	status int
	body   string
	seen   string
}

func (t *listTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(r.Body)
	t.seen = string(b)
	return &http.Response{StatusCode: t.status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(t.body)), Request: r}, nil
}

const listFixture = `{"data":{"MediaListCollection":{"lists":[
 {"entries":[{"media":{"id":10,"title":{"romaji":"Ten","english":"Ten EN"},"format":"TV","isAdult":false}},
             {"media":{"id":11,"title":{"romaji":"Eleven"},"format":"MOVIE","isAdult":false}}]},
 {"entries":[{"media":{"id":10,"title":{"romaji":"Ten","english":"Ten EN"},"format":"TV","isAdult":false}},
             {"media":{"id":12,"title":{"romaji":"Adult"},"format":"TV","isAdult":true}},
             {"media":null}]}
]}}}`

func TestAniListUserListDedupesAndDropsAdultAndNull(t *testing.T) {
	tr := &listTransport{status: 200, body: listFixture}
	svc := NewAnimeCatalogService(&http.Client{Transport: tr})
	got, err := svc.AniListUserList(context.Background(), "someone", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || got.Entries[0].ID != 10 || got.Entries[0].Title != "Ten EN" || got.Entries[1].ID != 11 {
		t.Fatalf("entries: %+v", got.Entries)
	}
	if got.Truncated {
		t.Fatal("not truncated")
	}
	if !strings.Contains(tr.seen, `"someone"`) {
		t.Fatalf("the user name must be sent as a variable: %s", tr.seen)
	}
}

func TestAniListUserListCapsTheResult(t *testing.T) {
	svc := NewAnimeCatalogService(&http.Client{Transport: &listTransport{status: 200, body: listFixture}})
	got, err := svc.AniListUserList(context.Background(), "someone", 1)
	if err != nil || len(got.Entries) != 1 || !got.Truncated {
		t.Fatalf("cap: %+v %v", got, err)
	}
}

func TestAniListUserListMapsMissingAndPrivateUsers(t *testing.T) {
	for status, want := range map[int]error{404: ErrAniListUserNotFound, 403: ErrAniListListPrivate} {
		svc := NewAnimeCatalogService(&http.Client{Transport: &listTransport{status: status, body: `{"errors":[{"message":"x"}]}`}})
		if _, err := svc.AniListUserList(context.Background(), "ghost", 100); !errors.Is(err, want) {
			t.Fatalf("status %d: want %v, got %v", status, want, err)
		}
	}
}

func TestAniListUserListRejectsBadNames(t *testing.T) {
	tr := &listTransport{status: 200, body: listFixture}
	svc := NewAnimeCatalogService(&http.Client{Transport: tr})
	for _, name := range []string{"", "a b", strings.Repeat("x", 40), "bad;name"} {
		if _, err := svc.AniListUserList(context.Background(), name, 100); !errors.Is(err, ErrAniListUserNotFound) {
			t.Fatalf("%q must be refused before any request, got %v", name, err)
		}
	}
	if tr.seen != "" {
		t.Fatal("no request may leave for an invalid name")
	}
}
