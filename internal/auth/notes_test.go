package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func putNotes(s *Service, c *http.Cookie, items []AnimeNote) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"notes": items})
	req := httptest.NewRequest("PUT", "/me/notes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(c)
	rr := httptest.NewRecorder()
	s.PutNotes(rr, req)
	return rr
}

func notesOf(rr *httptest.ResponseRecorder) map[int64]AnimeNote {
	var got struct {
		Notes []AnimeNote `json:"notes"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	out := map[int64]AnimeNote{}
	for _, n := range got.Notes {
		out[n.AnimeID] = n
	}
	return out
}

func TestNotesMergeNewestWinsAndRequireLogin(t *testing.T) {
	s := newTestService(t)
	c := registerForTest(t, s, "notes@example.com", "noter")
	now := time.Now().Unix()
	putNotes(s, c, []AnimeNote{{AnimeID: 5, Rating: 8, Note: "great", UpdatedAt: now}})
	rr := putNotes(s, c, []AnimeNote{
		{AnimeID: 5, Rating: 2, Note: "stale", UpdatedAt: now - 100},
		{AnimeID: 6, Rating: 0, Note: "only a note", UpdatedAt: now},
	})
	got := notesOf(rr)
	if got[5].Rating != 8 || got[5].Note != "great" || got[6].Note != "only a note" {
		t.Fatalf("newest must win: %+v", got)
	}
	// clearing is a newer write with empty values: it is kept as a tombstone so other devices drop it
	got = notesOf(putNotes(s, c, []AnimeNote{{AnimeID: 5, Rating: 0, Note: "", UpdatedAt: now + 10}}))
	if got[5].Rating != 0 || got[5].Note != "" || got[5].UpdatedAt != now+10 {
		t.Fatalf("a clear must propagate: %+v", got[5])
	}
	anon := httptest.NewRecorder()
	s.GetNotes(anon, httptest.NewRequest("GET", "/me/notes", nil))
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", anon.Code)
	}
}

func TestNotesRejectBadInput(t *testing.T) {
	s := newTestService(t)
	c := registerForTest(t, s, "bad@example.com", "badder")
	now := time.Now().Unix()
	for name, n := range map[string]AnimeNote{
		"zero id":       {AnimeID: 0, Rating: 5, UpdatedAt: now},
		"rating too hi": {AnimeID: 1, Rating: 11, UpdatedAt: now},
		"negative":      {AnimeID: 1, Rating: -1, UpdatedAt: now},
		"long note":     {AnimeID: 1, Note: strings.Repeat("é", maxNoteRunes+1), UpdatedAt: now},
		"future":        {AnimeID: 1, Rating: 3, UpdatedAt: now + 3600},
		"no stamp":      {AnimeID: 1, Rating: 3},
	} {
		if rr := putNotes(s, c, []AnimeNote{n}); rr.Code != http.StatusBadRequest {
			t.Fatalf("%s must be refused, got %d", name, rr.Code)
		}
	}
	if rr := putNotes(s, c, make([]AnimeNote, 201)); rr.Code != http.StatusBadRequest {
		t.Fatalf("too many items must be refused, got %d", rr.Code)
	}
}

func TestNotesAreExportedAndErasedWithTheAccount(t *testing.T) {
	s := newTestService(t)
	c := registerForTest(t, s, "exp@example.com", "exporter")
	putNotes(s, c, []AnimeNote{{AnimeID: 3, Rating: 7, Note: "keep", UpdatedAt: time.Now().Unix()}})
	var out struct {
		Notes []AnimeNote `json:"notes"`
	}
	_ = json.Unmarshal(authedGet(s.Export, c).Body.Bytes(), &out)
	if len(out.Notes) != 1 || out.Notes[0].Note != "keep" {
		t.Fatalf("export must carry the notes: %+v", out.Notes)
	}
	u := s.CurrentUser(reqWith(c))
	if err := s.store.DeleteUser(u.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.store.db.QueryRow(`SELECT COUNT(*) FROM anime_notes WHERE user_id = ?`, u.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("notes must go with the account: %d %v", n, err)
	}
}
