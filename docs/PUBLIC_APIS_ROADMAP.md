# Public APIs — later implementation roadmap

Catalog reviewed: [public-apis/public-apis](https://github.com/public-apis/public-apis) (Sports & Fitness, News, Weather, Events, Finance, Social, Open Data, Calendar, Geocoding). Date: 2026-08-18.

This is **not** a list of everything in that repo. It is the subset that would actually help NFL Splitboard: extra **splits/odds lenses**, **schedule/score/status**, **team identity**, **weather on outdoor games**, **news/injuries**, and **alerts**. Product work that needs no third-party API (divergence highlighting, history, filters) stays in [`docs/ROADMAP.md`](ROADMAP.md).

## How these would plug in

Splitboard is a Go collector + dashboard. New publishers belong under `internal/sources/`, implement `Source`, and register in `registry.go`. Enrichment that is **not** a split report (scores, weather, logos, news) should stay off the `GameSplits` market rows — attach as optional fields or a sibling collector so a failed weather call never blanks a splits card.

Keep the existing rule: **each publisher is its own lens, not a blended truth.** A paid odds API next to VSiN is another column, not a replacement.

Env-var pattern already used: `ACTION_NETWORK_COOKIE`. New keys should follow `LUMIFY_API_KEY`, `ODDS_API_KEY`, `NEWSAPI_KEY`, etc., never committed.

---

## Phase 1 — more betting lenses (highest leverage)

These map onto the current board: moneyline / spread / total, bet % vs money %, line, odds.

### 1. Lumify — public splits + odds + intelligence

| | |
|---|---|
| Catalog | [Lumify](https://lumify.ai/docs) — “scores, odds, betting splits & AI bet analysis” |
| Auth | `apiKey` (free 1,000 credits that never expire; trial key with no signup) |
| HTTPS / CORS | Yes / No |
| Docs | https://lumify.ai/docs · splits: `GET /v1/events/{id}/splits` |

**Why it fits.** This is the only catalog entry that already returns **ticket % vs handle %** for NFL, NBA, MLB, and NHL — the same shape as VSiN / DK Network / Action Network. Intelligence also scores “Betting Splits” as a signal, which is what the product roadmap wants to highlight as public-vs-sharp divergence.

**Implement later as.** A `Source` (`lumify`) that lists NFL (then NBA/NHL/MLB) events and maps bookmaker splits onto `MarketSplit` / `SideSplit`. Optional `ProInsight`-style attachment from `/intelligence` (grade/confidence), separate from Action PRO.

**Watch-outs.** CORS is No — server-side collect only (fine; we already scrape from Go). Odds cadence ~10 minutes. No player props / alternates yet. NCAAF has odds but **not** splits. Treat as another sample, not a merge of DK handle.

### 2. Bet Better — model win probability / fair odds

| | |
|---|---|
| Catalog | [Bet Better](https://betbetter.world/api/) — “Sports model win probabilities and fair odds” |
| Auth | None. CORS Yes. CC BY 4.0 (attribution required) |
| Feeds | `https://betbetter.world/nfl/picks.aspx?format=json` (also `/mlb/`, `/nba/`, `/nhl/`, `/ncaaf/`, `/ncaab/`, soccer leagues) |

**Why it fits.** Free, keyless JSON. Gives `modelProbabilityPct`, `fairOdds`, `confidence` (HIGH / LEAN / LONG-SHOT) for Head to Head, Spread, Total Points. That is a clean overlay against reported public %: “public is 72% on the favorite, model says 54%.” Same sports DK Network already shows.

**Implement later as.** A `Source` or enrichment join on team names + kickoff. Cache 15 minutes (their documented TTL). Show as a model chip, not as bet/money bars. Credit Bet Better in the footer.

**Watch-outs.** Deliberately **no** bookmaker prices. Empty in the offseason (same as Covers). Not betting advice — keep the existing sample disclaimer.

### 3. Odds-API.io — multi-book lines next to splits

| | |
|---|---|
| Catalog | [Odds-API](https://docs.odds-api.io) — 265+ books, 34 sports, REST + WebSocket |
| Auth | `apiKey`. Free: 100 req/h (500/day), 2 recreational books |
| HTTPS / CORS | Yes / Yes |

**Why it fits.** Splits without a line context are incomplete. A DraftKings vs FanDuel vs Pinnacle moneyline/spread/total on the same card answers “is the public side also the juiced side?” Historical odds would feed the steam / RLM work in the product roadmap **after** we have snapshot history.

**Implement later as.** Enrichment on contest key: attach `line` / `odds` from 1–2 books on free tier; do not poll WebSocket on the 15-minute refresh loop. Optional later: Circa vs DK delta (product roadmap item 4) using this instead of only VSiN’s two boards.

**Watch-outs.** Free tier is tiny — one collect of all NFL games can burn the daily budget. Cache aggressively; prefer TheRundown (Phase 4) if we need US sharps + prediction markets in one schema.

---

## Phase 2 — game state, identity, weather

Fixes “finished games linger” and “cards are just percentages” without changing how splits are collected.

### 4. TheSportsDB — schedule, scores, logos, venues

| | |
|---|---|
| Catalog | [TheSportsDB](https://www.thesportsdb.com/api.php) |
| Auth | `apiKey` (free demo key `3`; paid V2 for live scores / highlights) |
| HTTPS / CORS | Yes / Yes |

**Why it fits.** Crowd-sourced NFL (and NBA/NHL/MLB/soccer) events, team badges, stadium names, TV listings. We already ship static NFL logos; this is the path to **every sport DK Network lists** without maintaining an alias map by hand. Scores + status let the dashboard dim or drop completed contests (product roadmap item 2).

**Implement later as.** A lookup table keyed by normalized team name: `badge`, `stadium`, `status`, `score`. Not a `Source` for splits.

**Watch-outs.** Free tier is rate-limited and not live. Team-name matching will collide (Giants, Jets) — reuse `NFL_TEAM_ABBR` and add per-league maps (product roadmap item 7).

### 5. Open-Meteo or US NWS — kickoff weather

| | |
|---|---|
| Catalog | [Open-Meteo](https://open-meteo.com/) (no key, CORS Yes, CC BY 4.0, non-commercial cap) · [US Weather](https://www.weather.gov/documentation/services-web-api) (no key, CORS Yes) |
| Fallback | [Pirate Weather](https://pirateweather.net/en/latest/) if we want a Dark Sky–shaped JSON |

**Why it fits.** Outdoor NFL (and CFB) totals move with wind, precip, and temperature. A “13 mph wind, 28°F, snow likely” chip on the total row is more useful than a generic news blurb. Open-Meteo is the least-friction catalog option; NWS is the official US source if we already have stadium lat/lon.

**Implement later as.** After TheSportsDB (or a static stadium table) provides coordinates, one forecast call per unique venue per refresh. Attach to the contest, not to each source row.

**Watch-outs.** Indoor stadiums should skip. Open-Meteo commercial use needs a plan. NWS requires a unique User-Agent (we already send one in `internal/httpx`).

### 6. Nominatim — stadium coordinates (only if needed)

| | |
|---|---|
| Catalog | [Nominatim](https://nominatim.org/release-docs/latest/api/Overview/) |
| Auth | None. HTTPS Yes. CORS Yes |

**Why it fits.** One-shot geocode of “Lambeau Field, Green Bay” into lat/lon for Open-Meteo. Do **not** call this on every refresh — cache a `data/venues.json` (or bake coordinates). Respect Nominatim usage policy (identify the app in User-Agent).

---

## Phase 3 — multi-sport stats (after Action/VSiN/Covers go beyond NFL)

DK Network already paginates every sport. The other collectors are NFL-only. These APIs fill **schedule / standings / injuries** so non-NFL cards are not orphan percentages.

### 7. CollegeFootballData.com — NCAAF

| | |
|---|---|
| Catalog | [CollegeFootballData.com](https://collegefootballdata.com) |
| Auth | `apiKey` (free key via email; Bearer header) |
| Docs | https://api.collegefootballdata.com/documentation |

**Why it fits.** Best open American college football dataset: games, lines, SP+, recruiting, weather, team aliases. Directly supports sport-filter expansion and a CFB week picker analogous to NFL week.

### 8. NHL Records and Stats (official NHL feeds)

| | |
|---|---|
| Catalog | [NHL Records and Stats](https://gitlab.com/dword4/nhlapi) |
| Auth | None |
| Live base | `https://api-web.nhle.com/v1/` (`/schedule/now`, `/score/now`, `/standings/now`) |

**Why it fits.** Free, first-party schedule and scores. Community docs lag; hit `api-web.nhle.com`, not the retired `statsapi.web.nhl.com`. Use for NHL contest matching and finished-game prune.

### 9. balldontlie — NBA first, then NFL/MLB/NHL

| | |
|---|---|
| Catalog | [balldontlie](https://www.balldontlie.io) (listed as NBA-only; product now covers 20+ leagues) |
| Auth | None on the legacy NBA free path; current platform is `apiKey` with a free tier (5 req/min, 1 sport) |

**Why it fits.** Scores, injuries, odds, props, webhooks. Strongest for **NBA** cards. Paid All-Access is overkill unless we want injury webhooks instead of polling.

**Watch-outs.** Catalog description is stale. Confirm current ToS and which endpoints remain free before wiring NFL here vs Lumify/TheSportsDB.

### 10. NBA Stats / NBA Data — only as NBA fallbacks

| | |
|---|---|
| [NBA Stats](https://any-api.com/nba_com/nba_com/docs/API_Description) | Unofficial `stats.nba.com` — no key, brittle headers |
| [NBA Data](https://rapidapi.com/api-sports/api/api-nba/) | RapidAPI key; API-SPORTS family |

Prefer balldontlie. Keep these as backups if NBA grouping needs official team IDs.

**Skip:** [MLB Records and Stats](https://appac.github.io/mlb-data-api-docs/) — catalog marks HTTP-only / unknown CORS. Use balldontlie or TheSportsDB for MLB instead.

---

## Phase 4 — product expansions (after history + Phase 1)

Matches the “Later” table in the product roadmap: props, news, prediction markets, alerts.

### 11. PropLine — player props

| | |
|---|---|
| Catalog | [PropLine](https://prop-line.com) |
| Auth | `apiKey`. Free 1,000 req/day |
| HTTPS | Yes |

**Why it fits.** Graded player-prop odds across books + exchanges. NFL/NCAAF live odds exist now; graded NFL props start 2026 season. This is the right vendor **if** we add a props view — do not jam props into the three main markets.

### 12. TheRundown — US books + prediction markets, one schema

| | |
|---|---|
| Catalog | [TheRundown](https://therundown.io/) |
| Auth | `apiKey`. Free tier exists; paid for history / websocket |
| Coverage | NFL sport ID `2`; DK `19`, FanDuel `23`, Pinnacle `3`; also Kalshi / Polymarket |

**Why it fits.** Stronger **US sportsbook** set than Odds-API’s free recreational pair, plus prediction-market prices in the same event/market tree. Scores + schedules can replace a TheSportsDB paid upgrade. Choose **either** Odds-API **or** TheRundown as the odds backbone — do not run both on the 15-minute loop.

### 13. Dino.markets — Kalshi / Polymarket only

| | |
|---|---|
| Catalog | [Dino.markets](https://dino.markets/docs) (Finance) |
| Auth | `apiKey`. CORS No |

Use only if we want a dedicated “prediction market %” lens and are not already on TheRundown. Same disclaimer as splits: a contract price is not sportsbook handle.

### 14. News — injury / headline chip

Pick **one** search API, not three:

| Catalog | Auth | Notes |
|---|---|---|
| [NewsAPI](https://newsapi.org/) | `apiKey` | Simple `q=NFL+injury`; free tier is **dev-only** (localhost) |
| [GNews](https://gnews.io/) | `apiKey`, CORS Yes | Similar headline search; check commercial ToS |
| [The Guardian](http://open-platform.theguardian.com/) | `apiKey` | Stable, citable; NFL coverage is thinner |
| [New York Times](https://developer.nytimes.com/) | `apiKey` | Article search; stricter ToS |
| [Noozra](https://noozra.com/api) | None | RSS headlines; noisier, no key |

**Implement later as.** 1–3 headlines per matchup (`team A OR team B`, last 24h), shown under the card. Not a splits source. Guardian/NYT if we need redistributable excerpts; NewsAPI if the dashboard stays local.

### 15. Alerts — Discord / Telegram / Slack

| Catalog | Auth |
|---|---|
| [Discord](https://discord.com/developers/docs/intro) | OAuth / bot token / incoming webhook |
| [Telegram Bot](https://core.telegram.org/bots/api) | `apiKey` |
| [Slack](https://api.slack.com/) | OAuth |

**Why it fits.** Product roadmap already wants threshold alerts **after** snapshot history exists. Webhooks are enough; no need for a full bot at first. Fire on `|bet_pct - money_pct|` gap, RLM, or Lumify split divergence.

---

## Phase 5 — optional polish

| API | Catalog section | Use | Priority |
|---|---|---|---|
| [SeatGeek](https://platform.seatgeek.com/) | Events | Venue, listing volume as a crude public-interest proxy | Low |
| [Ticketmaster](http://developer.ticketmaster.com/products-and-docs/apis/getting-started/) | Events | Same as SeatGeek; pick one | Low |
| [The Calendar](https://the-calendar.net/api/) | Calendar | Static sports calendar JSON; NFL week labels if Action `week` is empty | Low |
| [Wikipedia](https://www.mediawiki.org/wiki/API:Main_page) / [Wikidata](https://www.wikidata.org/w/api.php?action=help) | Open Data | Team metadata, stadium, colors — TheSportsDB is easier | Low |
| [Reddit](https://www.reddit.com/dev/api) | Social | r/sportsbook chatter; noisy, OAuth, ToS-heavy | Skip unless asked |
| [Cloudbet](https://www.cloudbet.com/api/) | Sports | Live odds **only** — do **not** wire bet placement | Odds already covered |
| [SportScore](https://sportscore.com/developers/) | Sports | Live scores for soccer/basketball; TheSportsDB/balldontlie first | Low |

---

## Explicitly out of scope (from the same catalog)

These showed up under Sports & Fitness (or nearby) and should **not** be scheduled:

| API | Reason |
|---|---|
| API-FOOTBALL, Football-Data, Football Standings, Sportmonks Football, PlayerElo | Soccer (“football”), not NFL. Revisit only if we add a soccer **stats** layer on top of DK Network soccer splits |
| Canadian Football League | Adjacent league; no current collector |
| Oddsmagnet | UK book history, weak NFL |
| SuredBits | HTTP-only, CORS No |
| City Bikes, Fitbit, Strava, Wger, DiscGolf, Padel, Ergast/OpenF1, Squiggle (AFL) | Not betting splits / not this dashboard |
| IG (Finance) | CFD spreadbetting, not sportsbooks |
| Cloudbet bet-placement endpoints | This project reports public splits; it does not take wagers |

---

## Suggested order of attack

1. **Lumify splits source** — same domain model, immediate extra lens, free credits.
2. **Bet Better overlay** — zero auth, fair odds vs public % (pairs with product-roadmap divergence UI).
3. **TheSportsDB** — logos for non-NFL, schedule/status so finished games can drop.
4. **Open-Meteo + cached venues** — weather chip on NFL totals.
5. **One odds backbone** — Odds-API.io to prototype, or TheRundown if we want DK/FD/Pinnacle + prediction markets.
6. **CollegeFootballData + NHL web API** — when Action/VSiN/Covers leave NFL-only.
7. **NewsAPI or GNews** — injury headlines.
8. **PropLine** — only after a dedicated props view.
9. **Discord webhook alerts** — only after append-only history exists.

Do not implement 5–9 before snapshot history (`docs/ROADMAP.md` item 6). Odds movement and alerts without a time series will lie.

---

## Implementation sketch (when we start)

- New files: `internal/sources/lumify.go`, `internal/sources/betbetter.go`, optional `internal/enrich/{sportsdb,weather,odds}.go`.
- Register collectors in `registry.go`; gate paid ones on env vars so `go test ./...` stays offline (fixtures, same as existing parse tests).
- Health: `/api/sources` should list enrichment the same way as Covers (OK / error / games).
- Footer: name every live publisher; CC BY sources (Bet Better, Open-Meteo) need attribution.
- Bind address stays `127.0.0.1`. New keys are as sensitive as `ACTION_NETWORK_COOKIE`.

## Catalog drift

`public-apis` entries go stale (balldontlie is no longer NBA-only; NHL stats host moved). Re-check auth, ToS, and NFL coverage on the vendor docs the day an item is implemented — do not treat this file as a live SLA.
