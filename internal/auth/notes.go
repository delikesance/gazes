package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"unicode/utf8"
)

const (
	maxNoteRunes     = 500
	maxNotesPerBatch = 200
	maxNotesStored   = 2000
)

// AnimeNote is the viewer's private rating (1-10, 0 = none) and note for one anime. A write with
// no rating and no note is a tombstone: it stays so other devices learn the clear.
type AnimeNote struct {
	AnimeID   int64  `json:"anime_id"`
	Rating    int    `json:"rating"`
	Note      string `json:"note"`
	UpdatedAt int64  `json:"updated_at"`
}

// MergeNotes upserts each note only if it is newer than the stored one.
func (s *Store) MergeNotes(userID int64, items []AnimeNote) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, n := range items {
		if _, err := tx.Exec(`INSERT INTO anime_notes(user_id, anime_id, rating, note, updated_at) VALUES(?,?,?,?,?)
			ON CONFLICT(user_id, anime_id) DO UPDATE SET rating=excluded.rating, note=excluded.note, updated_at=excluded.updated_at
			WHERE excluded.updated_at > anime_notes.updated_at`, userID, n.AnimeID, n.Rating, n.Note, n.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListNotes(userID int64) ([]AnimeNote, error) {
	rows, err := s.db.Query(`SELECT anime_id, rating, note, updated_at FROM anime_notes WHERE user_id = ? ORDER BY updated_at DESC LIMIT ?`, userID, maxNotesStored)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AnimeNote{}
	for rows.Next() {
		var n AnimeNote
		if err := rows.Scan(&n.AnimeID, &n.Rating, &n.Note, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// GetNotes lists the viewer's private notes and ratings.
func (s *Service) GetNotes(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	notes, err := s.store.ListNotes(u.ID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
}

// PutNotes merges client notes (newest wins) and returns the merged state.
func (s *Service) PutNotes(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !s.sameOrigin(r) {
		fail(w, http.StatusForbidden, "forbidden")
		return
	}
	var body struct {
		Notes []AnimeNote `json:"notes"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 256<<10)).Decode(&body) != nil || len(body.Notes) > maxNotesPerBatch {
		fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	now := s.now().Unix()
	for _, n := range body.Notes {
		if n.AnimeID <= 0 || n.Rating < 0 || n.Rating > 10 || n.UpdatedAt <= 0 || n.UpdatedAt > now+300 ||
			!utf8.ValidString(n.Note) || utf8.RuneCountInString(n.Note) > maxNoteRunes {
			fail(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	if err := s.store.MergeNotes(u.ID, body.Notes); err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	s.GetNotes(w, r)
}
