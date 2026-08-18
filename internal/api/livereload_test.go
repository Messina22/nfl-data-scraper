package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	body := rec.Body.String()
	if !strings.Contains(body, "EventSource") {
		t.Error("script does not open an EventSource")
	}
	if !strings.Contains(body, "hello") {
		t.Error("script does not handle the hello event (boot ID tracking)")
	}
	if !strings.Contains(body, "location.reload") {
		t.Error("script does not reload the page")
	}
}

// TestHandleLiveReloadStreamsHelloAndReload drives the SSE handler end to end
// against a temp directory: it must emit a hello frame carrying the boot ID
// on connect, and a reload frame once a file in the watched directory changes.
func TestHandleLiveReloadStreamsHelloAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.js")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newTestServer(t, true)
	s.staticDir = dir
	s.bootID = "test-boot-id"
	h := s.Handler()

	// Bound the request context: httptest.NewRecorder() satisfies http.Flusher
	// and httptest.NewRequest's context never cancels on its own, so without
	// this the handler's poll loop would spin forever and hang the test binary.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/livereload", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(rec, req)
	}()

	// Let the handler emit its hello frame before touching the file.
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(path, []byte("bbbb"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Give the 300ms poll loop time to notice the change and emit reload,
	// then unblock the handler.
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not return after context cancellation")
	}

	// rec.Body is not goroutine-safe, so only read it after the handler
	// returns and the goroutine above has stopped writing to it.
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: hello\ndata: test-boot-id\n\n") {
		t.Errorf("missing hello frame with boot ID:\n%s", body)
	}
	if !strings.Contains(body, "event: reload") {
		t.Errorf("missing reload frame after file change:\n%s", body)
	}
}

func TestNewBootIDIsUniquePerCall(t *testing.T) {
	if a, b := newBootID(), newBootID(); a == b {
		t.Errorf("boot IDs collided: %q", a)
	}
}
