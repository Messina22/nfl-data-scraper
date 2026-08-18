# Dev Hot Reload Design

**Date:** 2026-08-13  
**Status:** Approved (Approach A + wgo)

## Goal

Remove the rebuild-and-restart cycle from the dashboard development loop. Editing
`web/static/*` should update the browser with no Go restart; editing Go should rebuild,
restart, and refresh the browser automatically.

## Problem

`web/embed.go` compiles the dashboard assets into the binary via `//go:embed static/*`.
The running server therefore serves a compile-time snapshot, so a one-line CSS change is
invisible until the process is rebuilt and restarted. The dashboard has no build step
(347 lines of vanilla JS, no npm), and a warm `go build` takes 0.33s — so compilation is
not the bottleneck. The embed is.

## Decisions

- Keep Go. A backend rewrite would reimplement ~2,600 lines of working scraper logic
  (goquery selectors, Action Network v2 parsing with PRO auth, per-source merge) to save
  0.33s per build.
- Runtime `-dev` flag, not a build tag — keeps one binary and one build path.
- SSE over WebSocket: reload is strictly server to browser, and `EventSource` reconnects
  natively with no client code.
- Poll modtimes rather than add `fsnotify` — three files at 300ms is free, and `go.mod`
  keeps its single direct dependency.
- `/api/livereload` is registered only when `Dev` is true. `/__livereload.js` is always
  registered: the client snippet in Dev, an empty no-op otherwise, so the dashboard
  `<script>` tag does not 404.

## Components

### 1. Asset source switch — `web/embed.go`

```go
func Assets(dev bool) (fs.FS, error)
```

Returns `os.DirFS("web/static")` when `dev`, otherwise `fs.Sub(StaticFS, "static")`.
`StaticFS` and the `//go:embed` directive are unchanged. In Dev, `Server` serves and
watches the same directory (`staticDir`, defaulting to `web.StaticDir`).

### 2. `-dev` flag — `main.go`

Adds `-dev` (default false), passed through as `api.Server.Dev`. Missing
`web/static/index.html` is a hard startup error so a wrong working directory cannot
look like a running dashboard.

### 3. Live-reload endpoint — `internal/api/livereload.go` (new)

- `GET /api/livereload` — SSE stream, Dev only. Emits `retry: 500` and `hello` carrying
  the process boot ID on connect, then `reload` whenever a file under the static
  directory changes.
- `GET /__livereload.js` — the client snippet in Dev; empty JavaScript otherwise.

The server generates a random boot ID on first SSE connect. A 300ms ticker walks the
static directory recursively and stats each file (path, size, mtime); names starting
with `.` or `_` are skipped, matching `//go:embed static/*`, and nested directories
with those prefixes are not entered. If any included file's size, mtime, or the file
set itself differs from the previous scan, connected clients receive `reload`.

Same-size writes within a single filesystem timestamp tick cannot be distinguished.

### 4. Client snippet

Referenced from `index.html` as `<script src="/__livereload.js" defer></script>`.

It opens an `EventSource` to `/api/livereload` and calls `location.reload()` on a `reload`
event. It also records the first boot ID it sees and reloads if a later `hello` carries a
different one.

## Data flow

**Static edit:** save → poller sees new modtime → SSE `reload` → browser reloads → server
reads the file fresh from disk. No rebuild, no restart, in-memory store preserved.

**Go edit:** save → `wgo` rebuilds and restarts (~0.5s) → SSE connection drops →
`EventSource` reconnects automatically (retry 500ms) → new process reports a different
boot ID → client reloads.

The boot-ID comparison is what lets one channel serve both halves: browser reconnection is
native `EventSource` behavior. `retry: 500` is the one restart-specific protocol extra,
so a `wgo` bounce is noticed in half a second instead of the browser's ~3s default.

## Production safety

Without `-dev`: assets come from `go:embed`, `/api/livereload` is not registered, and
`/__livereload.js` is an empty no-op. The binary stays self-contained. Rewriting HTML
in Dev to inject the tag would avoid the extra request at the cost of response-rewriting
middleware; a 200 empty script was judged cleaner than a console 404.

## Testing

- `Assets(false)` serves the embedded `index.html`, including the live-reload script tag.
- The modtime poller reports a change after a file in a temp dir is written, including
  same-size edits (via mtime) and deletions, and ignores `.`/`_`-prefixed names and dirs.
- `Handler()` with `Dev: false` returns 404 for `/api/livereload` and 200 empty JS for
  `/__livereload.js` — the regression that would leak live reload into production.
- In Dev, disk assets are served with `Cache-Control: no-store` from the same directory
  the poller watches.

## Dev command

```bash
wgo -file '\.go$' run . -dev
```

`-file '\.go$'` scopes the watcher to Go sources. Without it `wgo` would also restart on
static-asset changes, defeating the no-restart path. Requires
`go install github.com/bokwoon95/wgo@latest`; documented in the README quick start.
