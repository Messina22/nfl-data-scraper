# Action Network PRO Connection Design

**Date:** 2026-08-13  
**Status:** Approved (Approach A)

## Goal

Authenticate the existing Action Network collector with a personal Action PRO browser session so paywalled public-betting splits populate, and attach model projections (lean / grade / edge) onto each Action Network game for the matchup dashboard.

## Decisions

- Extend the existing `action-network` source (no second source id)
- Auth via `ACTION_NETWORK_COOKIE` env var only (no email/password login)
- Soft-fail projections: if splits succeed but projections fail, still return splits without `pro_insights`
- UI shows Pro lean/grade/edge on the existing Action Network market blocks
- Without a cookie, keep unauthenticated behavior

## Endpoints

- Public betting: `https://api.actionnetwork.com/web/v1/scoreboard/publicbetting/nfl`
- Game projections: `https://api.actionnetwork.com/web/v1/scoreboard/gameprojections/nfl`

Projection fields on odds objects (when authenticated) include `*_proj`, `*_edge_pct`, and `*_edge_grade` for spread, moneyline, and totals.

## Components

- `internal/httpx.Client`: optional `Cookie` header
- `models.ProInsight` + `GameSplits.ProInsights`
- `sources.ActionNetwork`: cookie-aware collect + projections join by game id
- Dashboard: render `pro_insights` under each market heading

## Failure modes

| Case | Behavior |
|------|----------|
| No cookie | Current public behavior |
| Cookie + 401/403 or empty splits | Error asking to refresh cookie |
| Splits OK, projections fail | Return splits; omit `pro_insights` |

## Out of scope

Email/password login, MFA, disk-persisted sessions, personal bet history, Cloudflare bypass tooling.
