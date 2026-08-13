# NFL Splitboard

Collect and display **NFL betting splits** — percentage of **bets** and **money (handle)** on moneyline, spread, and over/under — from multiple publicly reported sources.

## Features

- Multi-source collectors with a shared `Source` interface
- Dashboard at `/` grouping the same matchup across sources
- JSON API at `/api/splits` (plus `/api/sources`, `/api/refresh`, `/api/health`)
- Snapshot persistence to `data/splits.json` (per-source merge keeps last-good data when a source fails)
- Periodic refresh (default every 15 minutes)
- Optional Action Network PRO session via `ACTION_NETWORK_COOKIE` for unlocked money % and projection grade/edge

## Sources currently wired

| Source | What it reports |
|--------|-----------------|
| **VSiN (DraftKings)** | Spread / total / moneyline **handle %** and **bets %** |
| **VSiN (Circa)** | Same markets from Circa’s reported board |
| **Action Network** | Public betting API (`bet %` / `money %`); with PRO cookie also attaches `pro_insights` (lean / grade / edge) from game projections |
| **Covers Consensus** | Contest consensus picks when Covers publishes NFL matchup rows |

Additional sources can be added under `internal/sources/` and registered in `registry.go`.

## Quick start

```bash
go run .                 # collect + serve dashboard on http://127.0.0.1:8080
go run . -collect-only   # one-shot scrape into data/splits.json
go run . -refresh 10m    # auto-refresh while serving (still localhost by default)
```

Bind address defaults to `127.0.0.1:8080`. Only use `-addr :8080` if you intentionally want LAN/public access — `/api/refresh` has no auth and will trigger outbound scrapes.

Environment overrides: `SPLITS_ADDR`, `SPLITS_DATA`, `ACTION_NETWORK_COOKIE`.

## Action Network PRO cookie

Without a cookie, Action Network often returns empty bet/money fields (paywall / preseason). To unlock Pro public-betting splits and projection leans:

1. Log into Action Network PRO in your browser.
2. Open DevTools → Network → pick any request to `api.actionnetwork.com`.
3. Copy the full request `Cookie` header value.
4. Export it for the collector (never commit this value):

```bash
export ACTION_NETWORK_COOKIE='paste-cookie-header-here'
go run .
```

When the cookie is set, the Action Network collector:

- Sends it on `.../web/v1/scoreboard/publicbetting/nfl`
- Also fetches `.../web/v1/scoreboard/gameprojections/nfl` and maps per-market lean / grade / edge into each game’s `pro_insights`
- Soft-fails projections: if splits populate but projections fail, splits still return
- Surfaces a clear “refresh ACTION_NETWORK_COOKIE” error on 401/403 or still-empty paywalled fields

Cookies expire; refresh the env var when the source starts failing auth.

## API

- `GET /api/splits` — latest snapshot (sources + game reports)
- `GET /api/sources` — per-source status
- `POST /api/refresh` — trigger a fresh collection
- `GET /api/health` — liveness

## Project layout

```
main.go                 # server / CLI entrypoint
internal/
  models/               # split domain types
  sources/              # one collector per publisher
  collect/              # concurrent collection
  store/                # JSON snapshot persistence
  api/                  # HTTP handlers
  httpx/                # shared HTTP client
web/static/             # dashboard UI
data/                   # written snapshots (gitignored)
```

## Notes

Reported splits differ by publisher sample (sportsbook handle vs contest picks vs consensus products). Treat each source as its own lens, not a single blended truth.
