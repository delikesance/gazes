package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gazes/gazes/internal/donations"
)

func TestDonationsAdmin(t *testing.T) {
	e := newUsersEnv(t, "2026-03-10T12:00:00Z")
	e.user(t, 2, "lea", 1)
	st, err := donations.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e.svc.SetDonations(st)

	csrf := func(r *http.Request) { asAdmin(r); r.Header.Set("X-Gazes-Admin", "1") }
	post := func(path, body string, mut func(*http.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if mut != nil {
			mut(req)
		}
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec
	}

	// Tokens (so the MCP) never see who gave what; anonymous callers neither.
	if c := e.get("/api/v1/admin/donations", e.asToken).Code; c != 403 {
		t.Errorf("token list: %d, want 403", c)
	}
	if c := e.get("/api/v1/admin/donations", nil).Code; c != 401 {
		t.Errorf("anonymous list: %d, want 401", c)
	}
	if c := post("/api/v1/admin/donations", `{"amount_cents":500}`, e.asToken).Code; c != 403 {
		t.Errorf("token add: %d, want 403", c)
	}
	if c := post("/api/v1/admin/donations", `{"amount_cents":500}`, asAdmin).Code; c != 403 {
		t.Errorf("add without csrf header: %d, want 403", c)
	}

	rec := post("/api/v1/admin/donations", `{"amount_cents":1250,"donor_label":"virement Léa","user_id":2,"visibility":"named","display_name":"Léa"}`, csrf)
	if rec.Code != 200 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	var added struct {
		Data donationRow `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &added)
	if added.Data.Pseudo == nil || *added.Data.Pseudo != "lea" || added.Data.Provider != "manual" || added.Data.Status != "settled" {
		t.Fatalf("added %+v", added.Data)
	}
	for _, bad := range []string{`{"amount_cents":0}`, `{"amount_cents":500,"visibility":"named","display_name":"<b>"}`, `{"amount_cents":500,"user_id":2,"x":1}`} {
		if c := post("/api/v1/admin/donations", bad, csrf).Code; c != 400 {
			t.Errorf("%s: %d, want 400", bad, c)
		}
	}

	d := e.data(t, "/api/v1/admin/donations", asAdmin)
	rows := d["donations"].([]any)
	sum := d["summary"].(map[string]any)
	if len(rows) != 1 || sum["all_cents"].(float64) != 1250 || sum["count"].(float64) != 1 {
		t.Fatalf("list %v", d)
	}
	if rows[0].(map[string]any)["donor_label"] != "virement Léa" {
		t.Fatalf("admin does not see the label: %v", rows[0])
	}

	id := added.Data.ID
	if c := post("/api/v1/admin/donations/"+id+"/link", `{"user_id":99}`, csrf).Code; c != 404 {
		t.Errorf("link unknown account: %d, want 404", c)
	}
	if c := post("/api/v1/admin/donations/nope/link", `{"user_id":null}`, csrf).Code; c != 404 {
		t.Errorf("link unknown donation: %d, want 404", c)
	}
	if c := post("/api/v1/admin/donations/"+id+"/link", `{"user_id":null}`, csrf).Code; c != 200 {
		t.Errorf("unlink: %d", c)
	}
	if c := post("/api/v1/admin/donations/"+id+"/visibility", `{"visibility":"anonymous"}`, csrf).Code; c != 200 {
		t.Errorf("hide: %d", c)
	}
	if names, _ := st.PublicNames(t.Context(), 10); len(names) != 0 {
		t.Errorf("moderated name still public: %v", names)
	}
	if c := e.get("/api/v1/admin/donations?status=bogus", asAdmin).Code; c != 400 {
		t.Errorf("bad status: %d", c)
	}
}
