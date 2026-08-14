# Dev Hot Reload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Editing `web/static/*` updates the browser with no Go restart; editing Go rebuilds, restarts, and refreshes the browser automatically.

**Architecture:** A `-dev` flag swaps the `go:embed` asset filesystem for `os.DirFS`, so the server reads the dashboard from disk. A dev-only SSE endpoint polls the static directory and pushes a `reload` event to the browser. The same stream carries a per-process boot ID, so when `wgo` restarts the server after a Go edit, the browser's automatic `EventSource` reconnect sees a new ID and reloads. Production keeps the embedded assets and registers neither dev route.

**Tech Stack:** Go 1.22+ stdlib only (`embed`, `io/fs`, `net/http`, `path/filepath`), vanilla JS `EventSource`, `wgo` as an external dev tool.

**Spec:** `docs/superpowers/specs/2026-08-13-hot-reload-design.md`

## Global Constraints

- **No new module dependencies.** `go.mod` must keep exactly one direct dependency (`github.com/PuerkitoBio/goquery v1.9.2`). `wgo` is installed as a user tool via `go install`, never added to `go.mod`.
- **Production behavior is unchanged.** Without `-dev`: assets come from `//go:embed static/*`, and neither `/api/livereload` nor `/__livereload.js` is registered.
- **Tests use the stdlib `testing` package only** — no testify. Match the existing style in `internal/store/merge_test.go`.
- **Poll interval is 300ms.** Fingerprint compares path, size, and modtime, walking `web/static` recursively.
- **Static asset path is `web/static`**, resolved relative to the process working directory (repo root).

---

### Task 1: Asset source switch and `-dev` flag

Serves the dashboard from disk when `-dev` is set, from the embedded FS otherwise. After this task, editing `styles.css` and restarting shows the change — the rebuild is no longer required to pick up asset *content*.

**Files:**
- Modify: `web/embed.go`
- Create: `web/embed_test.go`
- Modify: `internal/api/server.go:18-40` (add `Dev` field, use `web.Assets`)
- Modify: `main.go:17-22` (add `-dev` flag), `main.go:42-46` (pass it through)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `web.StaticDir` — `const StaticDir = "web/static"`
  - `web.DevAssets(dir string) fs.FS`
  - `web.Assets(dev bool) (fs.FS, error)`
  - `api.Server.Dev bool` field

- [ ] **Step 1: Write the failing tests**

Create `web/embed_test.go`:

```go
package web

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetsEmbeddedServesIndex(t *testing.T) {
	fsys, err := Assets(false)
	if err != nil {
		t.Fatalf("Assets(false): %v", err)
	}
	b, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !strings.Contains(string(b), "NFL Splitboard") {
		t.Error("embedded index.html missing brand text")
	}
}

func TestDevAssetsReadsCurrentDiskContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "styles.css")
	if err := os.WriteFile(path, []byte("body{color:red}"), 0o644); err != nil {
		t.Fatal(err)
	}

	fsys := DevAssets(dir)
	if b, err := fs.ReadFile(fsys, "styles.css"); err != nil || string(b) != "body{color:red}" {
		t.Fatalf("first read: %q, %v", b, err)
	}

	// The whole point of dev mode: a later edit is visible without rebuilding.
	if err := os.WriteFile(path, []byte("body{color:blue}"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(fsys, "styles.css")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "body{color:blue}" {
		t.Errorf("got %q, want the edited contents", b)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./web/ -v`
Expected: FAIL — `undefined: Assets`, `undefined: DevAssets`

- [ ] **Step 3: Implement the asset switch**

Replace the contents of `web/embed.go`:

```go
package web

import (
	"embed"
	"io/fs"
	"os"
)

// StaticFS holds the dashboard assets.
//
//go:embed static/*
var StaticFS embed.FS

// StaticDir is the on-disk source for dashboard assets in dev mode,
// relative to the process working directory (the repo root).
const StaticDir = "web/static"

// DevAssets serves dashboard assets straight from a directory on disk, so
// edits are visible without rebuilding the binary.
func DevAssets(dir string) fs.FS {
	return os.DirFS(dir)
}

// Assets returns the filesystem the dashboard is served from: live from disk
// in dev mode, otherwise the copy embedded at build time.
func Assets(dev bool) (fs.FS, error) {
	if dev {
		return DevAssets(StaticDir), nil
	}
	return fs.Sub(StaticFS, "static")
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./web/ -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Add the `Dev` field and use `web.Assets` in the server**

In `internal/api/server.go`, add the field to the `Server` struct (after `RefreshInterval`):

```go
	// Dev serves assets from disk and enables live reload. Never set in production.
	Dev bool
```

Replace the static-file section of `Handler()` (currently lines 34-38):

```go
	assets, err := web.Assets(s.Dev)
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(assets)))
```

Then remove the now-unused `"io/fs"` import.

- [ ] **Step 6: Warn early if dev mode cannot see the assets**

`os.DirFS("web/static")` resolves against the working directory, so running from
elsewhere yields silent 404s. Add this to `Start()` in `internal/api/server.go`,
immediately after the `s.Store.Load()` block:

```go
	if s.Dev {
		if _, err := os.Stat(filepath.Join(web.StaticDir, "index.html")); err != nil {
			log.Printf("dev mode: cannot read %s/index.html — run from the repo root: %v", web.StaticDir, err)
		}
	}
```

Add `"os"` and `"path/filepath"` to the imports. This warns rather than exits, so a
misconfigured path is obvious without killing the server.

- [ ] **Step 7: Add the `-dev` flag**

In `main.go`, add alongside the other flags (after the `collectOnly` line):

```go
	dev := flag.Bool("dev", false, "serve assets from disk and enable live reload")
```

And add to the `api.Server` literal:

```go
		Dev:             *dev,
	}
```

- [ ] **Step 8: Verify the whole build and suite still pass**

Run: `go build ./... && go test ./...`
Expected: PASS, no build errors

- [ ] **Step 9: Commit**

```bash
git add web/embed.go web/embed_test.go internal/api/server.go main.go
git commit -m "Serve dashboard assets from disk under -dev."
```

---

### Task 2: Static directory change detector

A pure fingerprint function the SSE endpoint will poll. Kept separate from HTTP so it can be tested directly against a temp directory.

**Files:**
- Create: `internal/api/watch.go`
- Create: `internal/api/watch_test.go`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: `dirFingerprint(root string) string` — unexported, package `api`. Equal strings mean no change; any edit, addition, or deletion changes the result.

- [ ] **Step 1: Write the failing tests**

Create `internal/api/watch_test.go`:

```go
package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirFingerprintStableWithoutChanges(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if first, second := dirFingerprint(dir), dirFingerprint(dir); first != second {
		t.Errorf("fingerprint changed with no edit:\n%q\n%q", first, second)
	}
}

func TestDirFingerprintChangesOnEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "styles.css")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := dirFingerprint(dir)

	// Different length, so the check holds even at coarse modtime resolution.
	if err := os.WriteFile(path, []byte("bbbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	if second := dirFingerprint(dir); first == second {
		t.Error("fingerprint did not change after edit")
	}
}

func TestDirFingerprintDetectsNewFileInSubdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := dirFingerprint(dir)

	sub := filepath.Join(dir, "img")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "logo.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if second := dirFingerprint(dir); first == second {
		t.Error("fingerprint did not change after adding a file in a subdirectory")
	}
}

func TestDirFingerprintMissingDirIsEmpty(t *testing.T) {
	if got := dirFingerprint(filepath.Join(t.TempDir(), "nope")); got != "" {
		t.Errorf("got %q, want empty string for a missing directory", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/ -run TestDirFingerprint -v`
Expected: FAIL — `undefined: dirFingerprint`

- [ ] **Step 3: Implement the fingerprint**

Create `internal/api/watch.go`:

```go
package api

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// dirFingerprint summarizes every file under root by path, size, and modification
// time. Comparing two fingerprints detects edits, additions, and deletions in one
// string compare. Walk errors are ignored: a missing or unreadable directory
// yields a stable value rather than spurious reloads.
func dirFingerprint(root string) string {
	var sb strings.Builder
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		fmt.Fprintf(&sb, "%s:%d:%d\n", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return sb.String()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/api/ -run TestDirFingerprint -v`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/api/watch.go internal/api/watch_test.go
git commit -m "Add static directory fingerprint for change detection."
```

---

### Task 3: SSE live-reload endpoint and client snippet

Wires the fingerprint to the browser. This is the task that makes reload actually happen, and the one that must not leak into production.

**Files:**
- Create: `internal/api/livereload.go`
- Create: `internal/api/livereload_test.go`
- Modify: `internal/api/server.go` (`Server` struct: add `bootID`; `Handler()`: register dev routes, no-cache wrapper)
- Modify: `web/static/index.html:54` (add the script tag)

**Interfaces:**
- Consumes: `dirFingerprint(root string) string` (Task 2); `api.Server.Dev bool` and `web.StaticDir` (Task 1).
- Produces: `newBootID() string`; `(*Server).handleLiveReload`, `(*Server).handleLiveReloadScript`, `noCache(http.Handler) http.Handler` — all unexported.

- [ ] **Step 1: Write the failing tests**

Create `internal/api/livereload_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/ -run 'TestHandlerHides|TestLiveReload|TestNewBootID' -v`
Expected: FAIL — `undefined: newBootID`, and the script route returns 404 in dev

- [ ] **Step 3: Implement the live-reload endpoint**

Create `internal/api/livereload.go`:

```go
package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"nfl-data-scraper/web"
)

// liveReloadPoll is how often the server restats the static directory.
const liveReloadPoll = 300 * time.Millisecond

// liveReloadScript reloads the page on a static-asset change, and also when the
// boot ID changes — which happens when wgo rebuilds and restarts the server after
// a Go edit. EventSource reconnects on its own, so the restart needs no extra code.
const liveReloadScript = `(() => {
  let bootID = null;
  const es = new EventSource("/api/livereload");
  es.addEventListener("hello", (e) => {
    if (bootID === null) {
      bootID = e.data;
      return;
    }
    if (e.data !== bootID) location.reload();
  });
  es.addEventListener("reload", () => location.reload());
})();
`

// newBootID identifies one server process, so the browser can tell a reconnect
// to the same process from a reconnect to a freshly restarted one.
func newBootID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (s *Server) handleLiveReloadScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, liveReloadScript)
}

func (s *Server) handleLiveReload(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")

	fmt.Fprintf(w, "event: hello\ndata: %s\n\n", s.bootID)
	flusher.Flush()

	ticker := time.NewTicker(liveReloadPoll)
	defer ticker.Stop()
	prev := dirFingerprint(web.StaticDir)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			cur := dirFingerprint(web.StaticDir)
			if cur == prev {
				continue
			}
			prev = cur
			fmt.Fprint(w, "event: reload\ndata: 1\n\n")
			flusher.Flush()
		}
	}
}

// noCache stops the browser reusing a cached asset after an edit.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Register the dev routes**

In `internal/api/server.go`, add to the `Server` struct (after the `Dev` field):

```go
	bootID string
```

Then in `Handler()`, insert after the four `/api/...` route registrations and before
the asset section from Task 1:

```go
	if s.Dev {
		if s.bootID == "" {
			s.bootID = newBootID()
		}
		mux.HandleFunc("/api/livereload", s.handleLiveReload)
		mux.HandleFunc("/__livereload.js", s.handleLiveReloadScript)
	}
```

And wrap the file server so dev responses are never cached — replace the
`mux.Handle("/", ...)` line from Task 1 with:

```go
	var files http.Handler = http.FileServer(http.FS(assets))
	if s.Dev {
		files = noCache(files)
	}
	mux.Handle("/", files)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/api/ -v`
Expected: PASS (all tests, including the four from Task 2)

- [ ] **Step 6: Add the script tag to the dashboard**

In `web/static/index.html`, add immediately after line 54 (`<script src="/app.js"></script>`):

```html
  <script src="/__livereload.js" defer></script>
```

In production this 404s once and nothing runs — the accepted trade recorded in the spec.

- [ ] **Step 7: Verify the full suite passes**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 8: Manually verify the no-restart path**

```bash
go run . -dev
```

Open `http://127.0.0.1:8080`, then edit `web/static/styles.css` (change any color) and
save. The browser should reload on its own within ~300ms and show the new color, with
no server restart in the terminal.

- [ ] **Step 9: Commit**

```bash
git add internal/api/livereload.go internal/api/livereload_test.go internal/api/server.go web/static/index.html
git commit -m "Add SSE live reload for dashboard assets in dev mode."
```

---

### Task 4: `wgo` dev command and documentation

Closes the loop for Go edits and makes the workflow discoverable.

**Files:**
- Modify: `README.md:25-35` (Quick start section)

**Interfaces:**
- Consumes: the `-dev` flag (Task 1) and live reload (Task 3).
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Verify `wgo` restarts only on Go changes**

```bash
go install github.com/bokwoon95/wgo@latest
wgo -file '\.go$' run . -dev
```

With the browser open, edit `web/static/styles.css` — the page reloads and the terminal
shows **no** restart. Then edit any `.go` file, e.g. add a blank line to
`internal/api/server.go` — the terminal shows a rebuild and the page reloads again.

`-file '\.go$'` is what keeps asset edits off the restart path; without it `wgo`
watches everything and every CSS change would restart the server.

- [ ] **Step 2: Document the dev loop in the README**

In `README.md`, replace the Quick start code block (lines 27-31) with:

````markdown
```bash
go run .                 # collect + serve dashboard on http://127.0.0.1:8080
go run . -collect-only   # one-shot scrape into data/splits.json
go run . -refresh 10m    # auto-refresh while serving (still localhost by default)
go run . -dev            # serve assets from disk with browser live reload
```

### Hot reload

```bash
go install github.com/bokwoon95/wgo@latest   # once
wgo -file '\.go$' run . -dev
```

Editing `web/static/*` reloads the browser with no server restart — `-dev` serves the
dashboard from disk instead of the embedded copy. Editing Go rebuilds and restarts,
and the browser reloads when it reconnects. Keep `-file '\.go$'`: without it `wgo`
restarts on asset edits too, which defeats the no-restart path.

`-dev` resolves `web/static` relative to the working directory, so run it from the
repo root. Production builds are unaffected — assets stay embedded and the live-reload
routes are not registered.
````

- [ ] **Step 3: Verify the documented commands actually work**

Run each command from the README block and confirm it starts without error:

```bash
go run . -dev -refresh 0
```
Expected: logs `splits dashboard listening on http://127.0.0.1:8080` with no dev-mode
asset warning. Stop with Ctrl-C.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "Document the hot reload dev loop."
```

---

## Verification

After all tasks:

```bash
go build ./... && go test ./...
git diff --stat main
grep -c PuerkitoBio go.mod    # must be 1 — no new dependencies
```

Confirm production is untouched by building and running without `-dev`: assets serve
from the embedded FS, and `/api/livereload` and `/__livereload.js` both return 404.
