# Splitboard — next features and improvements

Prioritized backlog for NFL Splitboard after the current collectors (DraftKings Network, VSiN DK/Circa, Action Network, Covers), contest-grouped dashboard, Action PRO insights, and last-good snapshot merge.

In flight: [PR #12](https://github.com/Messina22/nfl-data-scraper/pull/12) (bet vs money divergence). Do not duplicate that work.

---

## Now — highest leverage on data we already collect

These can ship without new publishers. The model already has `bet_pct`, `money_pct`, `line`, `odds`, `num_bets`, `season`/`week`, and per-source `FetchedAt`.

### 1. Highlight bet % vs money % divergence — shipped

Classic “public vs sharp” signal: lopsided tickets on one side, handle on the other. Dashboard flags sides where `|bet_pct - money_pct|` ≥ 10 (strong ≥ 15), with a Gaps filter and Largest-gap sort. Handle-heavy money bars recolor when they disagree with tickets.

### 2. Hide finished games and age stale reports

`MergeSave` keeps last-good rows forever. Kickoff windows include games started in the last 12 hours, but completed contests and failed-source leftovers stay on the board.

- Drop (or dim) games whose `start_time` is well in the past
- Show “last good fetch” age on source pills, not only `collected_at`
- TTL or prune last-good rows after N hours so a dead source does not freeze a week-old card

### 3. Team search, NFL week, and persist filters

Action Network already fills `season` / `week` / `season_type`; the UI never uses them. Sport defaults to **All sports**, which is noisy now that DK Network covers every board.

- Text filter on team name / abbr
- NFL week (and preseason vs regular) selector
- Default sport to NFL (or remember last sport/source/window in `localStorage`)

### 4. Cross-source disagreement on a matchup card

Cards already stack every source under one contest (`contestKey` in `web/static/app.js`). Disagreement is still visual-only.

- When two sources on the same market lean opposite ways (or differ by >X pts), badge the row
- Show Circa vs DraftKings (VSiN) as a small delta, since those two are the same product family

### 5. Small dashboard polish

- DraftKings Network has no entry in `SOURCE_ICON` (Action / VSiN / Covers do)
- Sample-type chip: sportsbook handle vs Covers contest picks (README already warns these are different populations)
- Sort: kickoff (current) | most lopsided | most bets tracked | biggest source disagreement
- Health endpoint per-source OK/error — **done.** `GET /api/health` still returns HTTP 200 (liveness). Body includes each collector’s `ok` / `error` / `games`; overall `ok` is false only when every source failed (Covers offseason `ok: false` does not fail the probe)

---

## Next — new depth in the same product

### 6. Historical snapshots (the prerequisite for steam / RLM)

Store is a single `data/splits.json`. There is no series.

- Append-only history (SQLite or dated JSON) keyed by source + contest + market
- Chart bet % / money % / line over the refresh interval
- Reverse line movement: line goes one way, public the other
- Steam: sudden handle jump between ticks

Without this, alerts and “what changed since this morning” are guesswork.

### 7. Multi-sport on Action, VSiN, and Covers

DK Network already paginates every sport on their board. The other three collectors are hardcoded to NFL (`/nfl`, `sport=NFL`, Covers `/nfl/overall`). The dashboard sport filter is ready.

- Action Network: same v2 scoreboard URLs with `mlb` / `nba` / `nhl` (and college if the API allows)
- VSiN: `sport=` already a query param
- Covers: consensus URLs per league
- Non-NFL team matching: `NFL_TEAM_ABBR` is NFL-only on purpose (so CLE does not become the Browns). MLB/NBA/NHL need their own alias maps or grouping will miss rematches across sources

### 8. Action Network PRO session UX

Token is a manually copied JWT in `ACTION_NETWORK_COOKIE`. Expiry is a source error string.

- Surface “PRO connected” vs “token missing/expired” in the status bar
- Document (or script) a less painful refresh path — still no email/password login (see design spec out of scope)
- Optional: persist token in a local env file that is gitignored, not only the shell

### 9. Protect and observe the server

`/api/refresh` has no auth. README already says not to bind publicly.

- Shared secret / localhost-only refresh
- Retries and backoff in `internal/httpx` (one-shot GET today)
- Structured collect logs (source, duration, games, error) instead of `log.Printf` only
- Optional Docker / systemd unit once the dashboard is meant to stay up

---

## Later — product expansions

Worth doing after history + divergence are in, not before.

| Idea | Why wait |
|------|----------|
| Threshold alerts (Discord / ntfy / email) | Needs history + a stable contest key |
| Player props / alternate lines | Different pages and models; main three markets are still incomplete as a *view* |
| Blended “consensus” % across sources | Explicitly not a single truth today; only add as an opt-in lens with a disclaimer |
| CSV / JSON export of a window | Easy once filters are durable |
| Dark mode, PWA, kickoff countdown | Polish; not blocked on data |
| Injuries, news, DFS overlap | Different product |

Third-party APIs that could feed those later items (extra splits lenses, odds, scores, weather, news, alerts) are listed separately in [`docs/PUBLIC_APIS_ROADMAP.md`](PUBLIC_APIS_ROADMAP.md), sourced from [public-apis/public-apis](https://github.com/public-apis/public-apis). Do not start that list before the UI/history work above; Lumify + Bet Better are the first catalog entries worth wiring.

---

## Suggested order of attack

1. Divergence highlighting + sort/filter (pure UI on current snapshot) — shipped
2. Finished-game prune + stale source age
3. Team / week / default-NFL filters; DK icon; sample-type chips
4. Snapshot history
5. Action / VSiN / Covers for MLB–NBA–NHL
6. Refresh auth + httpx retries
7. Alerts on top of history

Keep treating each publisher as its own lens. New features should make disagreement easier to see, not hide it.
