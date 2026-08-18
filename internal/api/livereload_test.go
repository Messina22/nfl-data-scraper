package api

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
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

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// The regression that matters: live reload must never reach production.
func TestHandlerHidesLiveReloadSSEWithoutDev(t *testing.T) {
	rec := get(newTestServer(t, false).Handler(), "/api/livereload")
	if rec.Code != http.StatusNotFound {
		t.Errorf("/api/livereload: got status %d, want 404", rec.Code)
	}
}

func TestLiveReloadScriptIsNoopWithoutDev(t *testing.T) {
	rec := get(newTestServer(t, false).Handler(), "/__livereload.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/javascript" {
		t.Errorf("Content-Type = %q, want text/javascript", ct)
	}
	body := rec.Body.String()
	if body != "" {
		t.Errorf("production script must be empty, got %q", body)
	}
	if strings.Contains(body, "EventSource") {
		t.Error("production script must not open EventSource")
	}
}

func TestLiveReloadScriptServedInDev(t *testing.T) {
	rec := get(newTestServer(t, true).Handler(), "/__livereload.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/javascript" {
		t.Errorf("Content-Type = %q, want text/javascript", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `new EventSource("/api/livereload")`) {
		t.Error("script does not open an EventSource to /api/livereload")
	}
	if !strings.Contains(body, `"hello"`) {
		t.Error("script does not handle the hello event (boot ID tracking)")
	}
	if !strings.Contains(body, "location.reload") {
		t.Error("script does not reload the page")
	}
}

func TestHandlerServesEmbeddedIndexWithoutDev(t *testing.T) {
	rec := get(newTestServer(t, false).Handler(), "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "NFL Splitboard") {
		t.Error("production handler did not serve the embedded dashboard")
	}
	if !strings.Contains(body, `src="/__livereload.js"`) {
		t.Error("embedded index.html is missing the live-reload script tag")
	}
	if rec.Header().Get("Cache-Control") == "no-store" {
		t.Error("production assets should not force no-store")
	}
}

func TestHandlerServesDiskAssetsInDev(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("from-disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "styles.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newTestServer(t, true)
	s.staticDir = dir
	h := s.Handler()

	index := get(h, "/")
	if index.Code != http.StatusOK {
		t.Fatalf("GET /: status %d, want 200", index.Code)
	}
	if !strings.Contains(index.Body.String(), "from-disk") {
		t.Errorf("GET /: got %q, want disk contents", index.Body.String())
	}

	css := get(h, "/styles.css")
	if css.Code != http.StatusOK {
		t.Fatalf("GET /styles.css: status %d, want 200", css.Code)
	}
	if css.Body.String() != "body{}" {
		t.Errorf("GET /styles.css: got %q, want body{}", css.Body.String())
	}
	if cc := css.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestStartDevFailsWhenIndexMissing(t *testing.T) {
	s := newTestServer(t, true)
	s.staticDir = t.TempDir()
	s.Addr = "127.0.0.1:0"
	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when index.html is missing")
	}
	if !strings.Contains(err.Error(), "index.html") {
		t.Errorf("got %v, want an error mentioning index.html", err)
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

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/livereload", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	r := bufio.NewReader(resp.Body)
	hello := readSSEUntil(t, r, "event: hello\ndata: test-boot-id")
	if !strings.Contains(hello, "retry: 500") {
		t.Errorf("missing retry: 500:\n%s", hello)
	}

	// Fingerprint is taken before hello is flushed, so this write is a
	// change from the handler's point of view.
	if err := os.WriteFile(path, []byte("bbbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	reload := readSSEUntil(t, r, "event: reload\ndata: 1")
	if reload == "" {
		t.Error("missing reload frame after file change")
	}
}

func TestHandlerGeneratesBootID(t *testing.T) {
	s := newTestServer(t, true)
	s.staticDir = t.TempDir()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/livereload", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	frame := readSSEUntil(t, bufio.NewReader(resp.Body), "event: hello\ndata: ")
	id := bootIDFromHello(frame)
	if id == "" {
		t.Fatalf("empty boot ID in hello frame:\n%s", frame)
	}
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(id) {
		t.Errorf("boot ID %q is not 16 hex chars", id)
	}
}

func TestNewBootIDIsUniquePerCall(t *testing.T) {
	if a, b := newBootID(), newBootID(); a == b {
		t.Errorf("boot IDs collided: %q", a)
	}
}

func readSSEUntil(t *testing.T, r *bufio.Reader, needle string) string {
	t.Helper()
	var got strings.Builder
	for {
		line, err := r.ReadString('\n')
		got.WriteString(line)
		if strings.Contains(got.String(), needle) {
			return got.String()
		}
		if err != nil {
			if err == io.EOF {
				t.Fatalf("eof waiting for %q:\n%s", needle, got.String())
			}
			t.Fatalf("waiting for %q: %v\n%s", needle, err, got.String())
		}
	}
}

func bootIDFromHello(frame string) string {
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: ")
		}
	}
	return ""
}
