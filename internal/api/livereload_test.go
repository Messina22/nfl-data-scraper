package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"nfl-data-scraper/internal/store"
)

func newTestServer(t *testing.T, dev bool) *Server {
	t.Helper()
	return &Server{
		Store: store.New(filepath.Join(t.TempDir(), "splits.json")),
		Dev:   dev,
	}
}

// The regression that matters: live reload must never reach production.
func TestHandlerHidesLiveReloadRoutesWithoutDev(t *testing.T) {
	h := newTestServer(t, false).Handler()
	for _, path := range []string{"/api/livereload", "/__livereload.js"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: got status %d, want 404", path, rec.Code)
		}
	}
}

func TestLiveReloadScriptServedInDev(t *testing.T) {
	h := newTestServer(t, true).Handler()
	req := httptest.NewRequest(http.MethodGet, "/__livereload.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "EventSource") {
		t.Error("script does not open an EventSource")
	}
}

func TestNewBootIDIsUniquePerCall(t *testing.T) {
	if a, b := newBootID(), newBootID(); a == b {
		t.Errorf("boot IDs collided: %q", a)
	}
}
