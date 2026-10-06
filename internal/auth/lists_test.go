package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func listCall(s *Service, h http.HandlerFunc, method string, c *http.Cookie, id int64, body any) *httptest.ResponseRecorder {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "/me/lists", rd)
	req.Header.Set("Content-Type", "application/json")
	if c != nil {
		req.AddCookie(c)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

type listView struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	AnimeIDs []int64 `json:"anime_ids"`
}

func listsOf(rr *httptest.ResponseRecorder) []listView {
	var out struct {
		Lists []listView `json:"lists"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return out.Lists
}

func TestListsCreateFillRenameDelete(t *testing.T) {
	s := newTestService(t)
	c := registerForTest(t, s, "lists@example.com", "lister")
	rr := listCall(s, s.CreateList, "POST", c, 0, map[string]string{"name": "  Pour le week-end  "})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body)
	}
	lists := listsOf(rr)
	if len(lists) != 1 || lists[0].Name != "Pour le week-end" || len(lists[0].AnimeIDs) != 0 {
		t.Fatalf("created list: %+v", lists)
	}
	id := lists[0].ID
	rr = listCall(s, s.UpdateListItems, "PUT", c, id, map[string]any{"add": []int64{5, 6, 5}})
	if got := listsOf(rr); rr.Code != 200 || len(got[0].AnimeIDs) != 2 {
		t.Fatalf("add (duplicates collapse): %d %s", rr.Code, rr.Body)
	}
	rr = listCall(s, s.UpdateListItems, "PUT", c, id, map[string]any{"remove": []int64{5}})
	if got := listsOf(rr); len(got[0].AnimeIDs) != 1 || got[0].AnimeIDs[0] != 6 {
		t.Fatalf("remove: %s", rr.Body)
	}
	rr = listCall(s, s.RenameList, "PUT", c, id, map[string]string{"name": "Classiques"})
	if got := listsOf(rr); got[0].Name != "Classiques" {
		t.Fatalf("rename: %s", rr.Body)
	}
	rr = listCall(s, s.DeleteList, "DELETE", c, id, nil)
	if got := listsOf(rr); rr.Code != 200 || len(got) != 0 {
		t.Fatalf("delete: %d %s", rr.Code, rr.Body)
	}
	var n int
	if err := s.store.db.QueryRow(`SELECT COUNT(*) FROM user_list_items WHERE list_id = ?`, id).Scan(&n); err != nil || n != 0 {
		t.Fatalf("items must go with their list: %d %v", n, err)
	}
}

func TestListsAreLoggedInOwnedAndBounded(t *testing.T) {
	s := newTestService(t)
	a := registerForTest(t, s, "a@example.com", "alpha1")
	b := registerForTest(t, s, "b@example.com", "bravo1")
	if rr := listCall(s, s.GetLists, "GET", nil, 0, nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rr.Code)
	}
	id := listsOf(listCall(s, s.CreateList, "POST", a, 0, map[string]string{"name": "Mine"}))[0].ID
	if rr := listCall(s, s.UpdateListItems, "PUT", b, id, map[string]any{"add": []int64{1}}); rr.Code != http.StatusNotFound {
		t.Fatalf("another user must not edit it: %d", rr.Code)
	}
	if rr := listCall(s, s.DeleteList, "DELETE", b, id, nil); rr.Code != http.StatusNotFound {
		t.Fatalf("another user must not delete it: %d", rr.Code)
	}
	for name, body := range map[string]map[string]string{"empty": {"name": "   "}, "long": {"name": strings.Repeat("x", maxListNameRunes+1)}} {
		if rr := listCall(s, s.CreateList, "POST", a, 0, body); rr.Code != http.StatusBadRequest {
			t.Fatalf("%s name: %d", name, rr.Code)
		}
	}
	if rr := listCall(s, s.UpdateListItems, "PUT", a, id, map[string]any{"add": []int64{0}}); rr.Code != http.StatusBadRequest {
		t.Fatalf("bad anime id: %d", rr.Code)
	}
	for i := 1; i < maxLists; i++ {
		if rr := listCall(s, s.CreateList, "POST", a, 0, map[string]string{"name": "L" + strconv.Itoa(i)}); rr.Code != http.StatusCreated {
			t.Fatalf("list %d: %d", i, rr.Code)
		}
	}
	if rr := listCall(s, s.CreateList, "POST", a, 0, map[string]string{"name": "one too many"}); rr.Code != http.StatusConflict {
		t.Fatalf("list cap: %d", rr.Code)
	}
}

func TestListsAreExportedAndErasedWithTheAccount(t *testing.T) {
	s := newTestService(t)
	c := registerForTest(t, s, "x@example.com", "exlist")
	id := listsOf(listCall(s, s.CreateList, "POST", c, 0, map[string]string{"name": "Keep"}))[0].ID
	listCall(s, s.UpdateListItems, "PUT", c, id, map[string]any{"add": []int64{42}})
	var out struct {
		Lists []listView `json:"lists"`
	}
	_ = json.Unmarshal(authedGet(s.Export, c).Body.Bytes(), &out)
	if len(out.Lists) != 1 || out.Lists[0].Name != "Keep" || len(out.Lists[0].AnimeIDs) != 1 {
		t.Fatalf("export: %+v", out.Lists)
	}
	u := s.CurrentUser(reqWith(c))
	if err := s.store.DeleteUser(u.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.store.db.QueryRow(`SELECT COUNT(*) FROM user_lists`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("lists must go with the account: %d %v", n, err)
	}
}
