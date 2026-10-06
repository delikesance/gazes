package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gazes/gazes/internal/metadata"
)

type importTransport struct {
	status int
	body   string
}

func (t importTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: t.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(t.body)), Request: r}, nil
}

func importRequest(t *testing.T, tr importTransport, user string) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{catalogService: metadata.NewAnimeCatalogService(&http.Client{Transport: tr})}
	rec := httptest.NewRecorder()
	s.HandleAniListImport(rec, httptest.NewRequest("GET", "/api/v1/import/anilist?user="+user, nil))
	return rec
}

func TestAniListImportReturnsTheEntries(t *testing.T) {
	body := `{"data":{"MediaListCollection":{"lists":[{"entries":[{"media":{"id":5,"title":{"romaji":"Five"},"format":"TV","isAdult":false}}]}]}}}`
	rec := importRequest(t, importTransport{status: 200, body: body}, "someone")
	var out struct {
		Entries []struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != 200 || len(out.Entries) != 1 || out.Entries[0].ID != 5 {
		t.Fatalf("import: %d %s", rec.Code, rec.Body)
	}
}

func TestAniListImportSeparatesMissingPrivateAndInvalidUsers(t *testing.T) {
	for _, tc := range []struct {
		status int
		user   string
		want   int
		code   string
	}{
		{404, "ghost", 404, "user_not_found"},
		{403, "secret", 403, "list_private"},
		{200, "bad%20name", 404, "user_not_found"},
	} {
		rec := importRequest(t, importTransport{status: tc.status, body: `{}`}, tc.user)
		if rec.Code != tc.want || !strings.Contains(rec.Body.String(), tc.code) {
			t.Fatalf("%s: %d %s", tc.user, rec.Code, rec.Body)
		}
	}
}
