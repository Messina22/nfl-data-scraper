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
- Dev routes are registered only when `Dev` is true. Production behavior is unchanged.

## Components

### 1. Asset source switch — `web/embed.go`

```go
func Assets(dev bool) (fs.FS, error)
```

Returns `os.DirFS("web/static")` when `dev`, otherwise `fs.Sub(StaticFS, "static")`.
`StaticFS` and the `//go:embed` directive are unchanged.

### 2. `-dev` flag — `main.go`

Adds `-dev` (default false), passed through as `api.Server.Dev`.

### 3. Live-reload endpoint — `internal/api/livereload.go` (new)

Registered only when `Dev` is true:

- `GET /api/livereload` — SSE stream. Emits `hello` carrying the process boot ID on
  connect, then `reload` whenever a file under `web/static` changes.
- `GET /__livereload.js` — the client snippet.

The server generates a random boot ID at process start. A 300ms ticker walks `web/static`
recursively and stats each file; if any modtime, size, or the file set itself differs from
the previous scan, connected clients receive `reload`. Walking recursively costs nothing at
the current three files and means a future subdirectory works without a code change.

### 4. Client snippet

Referenced from `index.html` as `<script src="/__livereload.js" defer></script>`.

It opens an `EventSource` to `/api/livereload` and calls `location.reload()` on a `reload`
event. It also records the first boot ID it sees and reloads if a later `hello` carries a
different one.

## Data flow

**Static edit:** save → poller sees new modtime → SSE `reload` → browser reloads → server
reads the file fresh from disk. No rebuild, no restart, in-memory store preserved.

**Go edit:** save → `wgo` rebuilds and restarts (~0.5s) → SSE connection drops →
`EventSource` reconnects automatically → new process reports a different boot ID → client
reloads.

The boot-ID comparison is what lets one channel serve both halves: browser reconnection is
native behavior, so a Go restart refreshes the page without any restart-specific code.

## Production safety

Without `-dev`: assets come from `go:embed`, neither dev route is registered, and the
binary stays self-contained. The only trace is one 404 for `/__livereload.js`. Rewriting
HTML in dev to inject the tag would avoid that 404 at the cost of response-rewriting
middleware; the 404 was judged the better trade.

## Testing

- `Assets(false)` serves the embedded `index.html`.
- The modtime poller reports a change after a file in a temp dir is written.
- `Handler()` with `Dev: false` returns 404 for `/api/livereload` and `/__livereload.js` —
  the regression that would leak live reload into production.

## Dev command

```bash
wgo -file '\.go$' run . -dev
```

`-file '\.go$'` scopes the watcher to Go sources. Without it `wgo` would also restart on
static-asset changes, defeating the no-restart path. Requires
`go install github.com/bokwoon95/wgo@latest`; documented in the README quick start.
