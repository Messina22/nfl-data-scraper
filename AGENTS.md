# AGENTS.md

## Cursor Cloud specific instructions

NFL Splitboard is a single Go web service (`main.go` + `internal/*`) that scrapes publicly reported betting splits and serves a dashboard at `/` plus a JSON API under `/api/*`. There is one service; the standard commands live in `README.md` and `.cursor/environment.json`.

Standard commands (Go 1.22.x; the toolchain auto-downloads the pinned version):

- Build: `go build ./...`
- Lint: `go vet ./...` (no golangci-lint config in the repo)
- Test: `go test ./...` (real tests live only in `internal/sources` and `internal/store`; all parsing is tested against inline HTML/JSON fixtures, so tests need no network)
- Run: `go run . -refresh 0` (serves on `http://127.0.0.1:8080`; `-refresh 0` disables the periodic auto-refresh, which is what the `splits-server` terminal in `.cursor/environment.json` uses)

Non-obvious caveats:

- On startup the server performs a live outbound scrape (VSiN, Action Network, DraftKings Network, Covers) whenever the on-disk snapshot has no games, so the first request can lag ~10-20s while collection runs. This requires network egress; if egress is restricted the dashboard will come up empty.
- `covers-consensus` normally reports `ok: false` with a "no matchup split rows" message during off/preseason — this is expected, not a setup failure. The other four sources returning games is the healthy signal.
- Snapshots persist to `data/splits.json` (gitignored). Delete it to force a fresh full scrape on next boot; otherwise `Start` skips the initial collect when games already exist.
- `/api/refresh` is a `POST` with no auth and triggers outbound scrapes. Keep the bind address on `127.0.0.1` (the default) unless LAN/public access is intentionally required.
- Action Network money % / projection fields stay empty without a PRO JWT. Set `ACTION_NETWORK_COOKIE` (raw JWT, no `Bearer` prefix) to unlock them; the bet % splits still populate without it.
