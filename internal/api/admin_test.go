package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/auth"
	"github.com/gazes/gazes/internal/config"
)

func TestAdminRoutesMounting(t *testing.T) {
	dir := t.TempDir()
	as, err := auth.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer as.Close()
	st, err := admin.Open(filepath.Join(dir, "admin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc, err := admin.NewService(st, filepath.Join(dir, "accounts.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	tests := []struct {
		name string
		opts []Option
		want int
	}{
		{"without WithAdmin", nil, http.StatusNotFound},
		{"with WithAdmin, logged out", []Option{WithAdmin(svc), WithLibraryUser(func(*http.Request) (int64, bool) { return 0, false })}, http.StatusUnauthorized},
		{"with WithAdmin, logged in but no auth service", []Option{WithAdmin(svc), WithLibraryUser(func(*http.Request) (int64, bool) { return 1, true })}, http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer(&config.Config{}, nil, nil, nil, nil, tc.opts...)
			rec := httptest.NewRecorder()
			s.Router().ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/admin/me", nil))
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
