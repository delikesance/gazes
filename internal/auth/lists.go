package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

const (
	maxLists         = 20
	maxListItems     = 500
	maxListNameRunes = 40
	maxListEdits     = 200
)

var (
	errListFull     = errors.New("list is full")
	errTooManyLists = errors.New("too many lists")
)

// UserList is one named, private collection of anime ids.
type UserList struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	AnimeIDs []int64 `json:"anime_ids"`
}

// ListsOf returns a user's lists in creation order, each with its anime (most recently added first).
func (s *Store) ListsOf(userID int64) ([]UserList, error) {
	rows, err := s.db.Query(`SELECT id, name FROM user_lists WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	out := []UserList{}
	for rows.Next() {
		l := UserList{AnimeIDs: []int64{}}
		if err := rows.Scan(&l.ID, &l.Name); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	items, err := s.db.Query(`SELECT i.list_id, i.anime_id FROM user_list_items i JOIN user_lists l ON l.id = i.list_id WHERE l.user_id = ? ORDER BY i.created_at DESC, i.anime_id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer items.Close()
	byID := map[int64]*UserList{}
	for i := range out {
		byID[out[i].ID] = &out[i]
	}
	for items.Next() {
		var listID, animeID int64
		if err := items.Scan(&listID, &animeID); err != nil {
			return nil, err
		}
		if l := byID[listID]; l != nil {
			l.AnimeIDs = append(l.AnimeIDs, animeID)
		}
	}
	return out, items.Err()
}

func (s *Store) CreateList(userID int64, name string, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM user_lists WHERE user_id = ?`, userID).Scan(&n); err != nil {
		return err
	}
	if n >= maxLists {
		return errTooManyLists
	}
	if _, err := tx.Exec(`INSERT INTO user_lists(user_id, name, created_at) VALUES(?,?,?)`, userID, name, now.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// ownsList reports whether the list exists and belongs to the user.
func ownsList(tx *sql.Tx, userID, listID int64) (bool, error) {
	var n int
	err := tx.QueryRow(`SELECT COUNT(*) FROM user_lists WHERE id = ? AND user_id = ?`, listID, userID).Scan(&n)
	return n > 0, err
}

func (s *Store) RenameList(userID, listID int64, name string) (bool, error) {
	res, err := s.db.Exec(`UPDATE user_lists SET name = ? WHERE id = ? AND user_id = ?`, name, listID, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) DeleteList(userID, listID int64) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM user_lists WHERE id = ? AND user_id = ?`, listID, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UpdateListItems removes then adds anime of one of the user's lists; found is false for a list that is not theirs.
func (s *Store) UpdateListItems(userID, listID int64, add, remove []int64, now time.Time) (found bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if ok, err := ownsList(tx, userID, listID); err != nil || !ok {
		return false, err
	}
	for _, id := range remove {
		if _, err := tx.Exec(`DELETE FROM user_list_items WHERE list_id = ? AND anime_id = ?`, listID, id); err != nil {
			return true, err
		}
	}
	for _, id := range add {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO user_list_items(list_id, anime_id, created_at) VALUES(?,?,?)`, listID, id, now.Unix()); err != nil {
			return true, err
		}
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM user_list_items WHERE list_id = ?`, listID).Scan(&n); err != nil {
		return true, err
	}
	if n > maxListItems {
		return true, errListFull
	}
	return true, tx.Commit()
}

func (s *Service) listsReply(w http.ResponseWriter, u *User, status int) {
	lists, err := s.store.ListsOf(u.ID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, map[string]any{"lists": lists})
}

// listUser resolves the signed-in user and, for writes, applies the same-origin guard.
func (s *Service) listUser(w http.ResponseWriter, r *http.Request, write bool) *User {
	u := s.currentUser(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return nil
	}
	if write && !s.sameOrigin(r) {
		fail(w, http.StatusForbidden, "forbidden")
		return nil
	}
	return u
}

func cleanListName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	return name, name != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= maxListNameRunes
}

func listID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil && id > 0
}

// GetLists returns the viewer's named lists.
func (s *Service) GetLists(w http.ResponseWriter, r *http.Request) {
	if u := s.listUser(w, r, false); u != nil {
		s.listsReply(w, u, http.StatusOK)
	}
}

// CreateList adds an empty named list and returns all lists.
func (s *Service) CreateList(w http.ResponseWriter, r *http.Request) {
	u := s.listUser(w, r, true)
	if u == nil {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	name, ok := cleanListName("")
	if json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body) == nil {
		name, ok = cleanListName(body.Name)
	}
	if !ok {
		fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	switch err := s.store.CreateList(u.ID, name, s.now()); {
	case errors.Is(err, errTooManyLists):
		fail(w, http.StatusConflict, "too_many_lists")
	case err != nil:
		fail(w, http.StatusInternalServerError, "server_error")
	default:
		s.listsReply(w, u, http.StatusCreated)
	}
}

// RenameList renames one of the viewer's lists.
func (s *Service) RenameList(w http.ResponseWriter, r *http.Request) {
	u := s.listUser(w, r, true)
	if u == nil {
		return
	}
	id, idOK := listID(r)
	var body struct {
		Name string `json:"name"`
	}
	name, nameOK := "", false
	if json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body) == nil {
		name, nameOK = cleanListName(body.Name)
	}
	if !idOK || !nameOK {
		fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	found, err := s.store.RenameList(u.ID, id, name)
	if err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	if !found {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	s.listsReply(w, u, http.StatusOK)
}

// DeleteList removes one of the viewer's lists with its items.
func (s *Service) DeleteList(w http.ResponseWriter, r *http.Request) {
	u := s.listUser(w, r, true)
	if u == nil {
		return
	}
	id, ok := listID(r)
	if !ok {
		fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	found, err := s.store.DeleteList(u.ID, id)
	if err != nil {
		fail(w, http.StatusInternalServerError, "server_error")
		return
	}
	if !found {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	s.listsReply(w, u, http.StatusOK)
}

// UpdateListItems adds and removes anime of one list and returns all lists.
func (s *Service) UpdateListItems(w http.ResponseWriter, r *http.Request) {
	u := s.listUser(w, r, true)
	if u == nil {
		return
	}
	id, idOK := listID(r)
	var body struct {
		Add    []int64 `json:"add"`
		Remove []int64 `json:"remove"`
	}
	if !idOK || json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body) != nil || len(body.Add) > maxListEdits || len(body.Remove) > maxListEdits {
		fail(w, http.StatusBadRequest, "invalid_request")
		return
	}
	for _, anime := range append(append([]int64{}, body.Add...), body.Remove...) {
		if anime <= 0 {
			fail(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	found, err := s.store.UpdateListItems(u.ID, id, body.Add, body.Remove, s.now())
	switch {
	case errors.Is(err, errListFull):
		fail(w, http.StatusConflict, "list_full")
	case err != nil:
		fail(w, http.StatusInternalServerError, "server_error")
	case !found:
		fail(w, http.StatusNotFound, "not_found")
	default:
		s.listsReply(w, u, http.StatusOK)
	}
}
