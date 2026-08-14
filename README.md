# NFL Splitboard

Collect and display **betting splits** — percentage of **bets** and **money (handle)** on moneyline, spread, and over/under — from multiple publicly reported sources. NFL remains covered by VSiN, Action Network, and Covers; **DraftKings Network** adds first-party splits across every sport on their board (NFL, MLB, NBA, NHL, college, soccer, and more).

## Features

- Multi-source collectors with a shared `Source` interface
- Dashboard at `/` grouping the same matchup across sources, with **sport** and source filters
- JSON API at `/api/splits` (plus `/api/sources`, `/api/refresh`, `/api/health`)
- Snapshot persistence to `data/splits.json` (per-source merge keeps last-good data when a source fails)
- Periodic refresh (default every 15 minutes)
- Optional Action Network PRO session via `ACTION_NETWORK_COOKIE` (JWT from the `authorization` request header) for unlocked money % and projection grade/edge

## Sources currently wired

| Source | What it reports |
|--------|-----------------|
| **DraftKings Network** | First-party Sportsbook **handle %** and **bets %** for spread / total / moneyline, collected for every sport listed on [their splits board](https://dknetwork.draftkings.com/draftkings-sportsbook-betting-splits/) (not MLB-only) |
| **VSiN (DraftKings)** | NFL spread / total / moneyline **handle %** and **bets %** |
| **VSiN (Circa)** | Same NFL markets from Circa’s reported board |
| **Action Network** | NFL public betting API (`bet %` / `money %`); with PRO cookie also attaches `pro_insights` (lean / grade / edge) from game projections |
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

## Action Network PRO auth

Without a session token, Action Network often returns empty bet/money fields (paywall / preseason). To unlock Pro public-betting splits and projection leans:

1. Log into Action Network PRO in your browser.
2. Open DevTools → Network → filter `publicbetting` → select the `api.actionnetwork.com` XHR.
3. Under **Headers → Request Headers**, copy the `authorization` value (raw JWT, no `Bearer` prefix).  
   Same value as cookie `AN_SESSION_TOKEN_V1` — the API does **not** use a Cookie header.
4. Export it for the collector (never commit this value):

```bash
export ACTION_NETWORK_COOKIE='eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...'
go run .
```

When the token is set, the Action Network collector:

- Sends it as the `Authorization` header on `.../web/v2/scoreboard/publicbetting/nfl?periods=event`
- Parses v2 `markets.<book>.event.{spread,total,moneyline}[].bet_info.{tickets,money}.percent`
- Also fetches `.../web/v2/scoreboard/gameprojections/nfl?periods=event` and maps lean / grade / edge into `pro_insights` when present
- Soft-fails projections: if splits populate but projections fail, splits still return
- Surfaces a clear “refresh ACTION_NETWORK_COOKIE” error on 401/403 or still-empty paywalled fields

Tokens expire; refresh the env var when the source starts failing auth.

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
