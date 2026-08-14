package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nfl-data-scraper/internal/httpx"
	"nfl-data-scraper/internal/models"
)

// ActionNetwork collects public betting percentages from Action Network's
// scoreboard API when the provider exposes bet/money split fields.
// When ACTION_NETWORK_COOKIE is set to the JWT from the browser's
// Authorization request header (AN_SESSION_TOKEN_V1), Pro fields unlock
// and game projections (grade/edge) are joined onto each game.
type ActionNetwork struct {
	client *httpx.Client
	token  string
}

func NewActionNetwork(token string) *ActionNetwork {
	c := httpx.New(45 * time.Second)
	c.Referer = "https://www.actionnetwork.com/nfl/public-betting"
	c.Origin = "https://www.actionnetwork.com"
	token = strings.TrimSpace(token)
	// Accept either the raw JWT or a mistaken "Bearer <jwt>" paste.
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	token = strings.TrimSpace(token)
	c.Authorization = token
	return &ActionNetwork{client: c, token: token}
}

func (a *ActionNetwork) ID() string   { return "action-network" }
func (a *ActionNetwork) Name() string { return "Action Network" }

const (
	// Live site uses v2. bookIds are optional; response includes consensus book 15 with public splits.
	actionPublicBettingURL   = "https://api.actionnetwork.com/web/v2/scoreboard/publicbetting/nfl?periods=event"
	actionGameProjectionsURL = "https://api.actionnetwork.com/web/v2/scoreboard/gameprojections/nfl?periods=event"
	actionPublicBettingPage  = "https://www.actionnetwork.com/nfl/public-betting"
	actionProjectionsPage    = "https://www.actionnetwork.com/nfl/projections"
	actionAuthExpiredMessage = "Action Network auth failed or token expired — refresh ACTION_NETWORK_COOKIE (JWT from the authorization request header)"
)

func (a *ActionNetwork) Collect(ctx context.Context) ([]models.GameSplits, error) {
	body, _, err := a.client.Get(ctx, actionPublicBettingURL)
	if err != nil {
		if a.token != "" && isHTTPAuthError(err) {
			return nil, fmt.Errorf("%s", actionAuthExpiredMessage)
		}
		return nil, err
	}

	var payload actionScoreboard
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode action network json: %w", err)
	}

	insightsByGame := map[int][]models.ProInsight{}
	if a.token != "" {
		if insights, perr := a.fetchProInsights(ctx); perr == nil {
			insightsByGame = insights
		}
		// Projection failures are soft: splits still return when populated.
	}

	now := time.Now().UTC()
	var out []models.GameSplits
	for _, g := range payload.Games {
		away, home := actionTeams(g)
		if away.FullName == "" || home.FullName == "" {
			continue
		}
		markets := actionMarketsFromGame(away, home, g)
		gs := models.GameSplits{
			SourceID:    a.ID(),
			SourceName:  a.Name(),
			Book:        "Action Network Consensus",
			ExternalID:  fmt.Sprintf("%d", g.ID),
			AwayTeam:    away.FullName,
			HomeTeam:    home.FullName,
			AwayAbbr:    away.Abbr,
			HomeAbbr:    home.Abbr,
			Season:      g.Season,
			Week:        g.Week,
			SeasonType:  g.Type,
			FetchedAt:   now,
			URL:         actionPublicBettingPage,
			Markets:     markets,
			ProInsights: insightsByGame[g.ID],
		}
		if g.NumBets > 0 {
			n := g.NumBets
			gs.NumBets = &n
		}
		if t, err := time.Parse(time.RFC3339, g.StartTime); err == nil {
			utc := t.UTC()
			gs.StartTime = &utc
		}
		if !hasAnySplit(gs) {
			continue
		}
		out = append(out, gs)
	}

	if len(out) == 0 {
		if a.token != "" {
			return nil, fmt.Errorf("%s (publicbetting returned %d games with empty bet/money fields)", actionAuthExpiredMessage, len(payload.Games))
		}
		return nil, fmt.Errorf("action network NFL scoreboard reachable (%d games) but bet/money split fields are empty (often paywalled or unavailable in preseason)", len(payload.Games))
	}
	return out, nil
}

func (a *ActionNetwork) fetchProInsights(ctx context.Context) (map[int][]models.ProInsight, error) {
	// Use projections page referer for this call.
	prev := a.client.Referer
	a.client.Referer = actionProjectionsPage
	defer func() { a.client.Referer = prev }()

	body, _, err := a.client.Get(ctx, actionGameProjectionsURL)
	if err != nil {
		return nil, err
	}
	var payload actionScoreboard
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode action network projections: %w", err)
	}
	out := make(map[int][]models.ProInsight, len(payload.Games))
	for _, g := range payload.Games {
		away, home := actionTeams(g)
		insights := actionProInsightsFromGame(away, home, g)
		if len(insights) == 0 {
			// Legacy v1 flat odds shape, if still present.
			insights = actionProInsights(away, home, pickActionOdds(g.Odds))
		}
		if len(insights) > 0 {
			out[g.ID] = insights
		}
	}
	return out, nil
}

func isHTTPAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP 401") || strings.Contains(msg, "HTTP 403")
}

type actionScoreboard struct {
	Games []actionGame `json:"games"`
}

type actionGame struct {
	ID         int                          `json:"id"`
	StartTime  string                       `json:"start_time"`
	AwayTeamID int                          `json:"away_team_id"`
	HomeTeamID int                          `json:"home_team_id"`
	Season     int                          `json:"season"`
	Week       int                          `json:"week"`
	Type       string                       `json:"type"`
	NumBets    int                          `json:"num_bets"`
	Teams      []actionTeam                 `json:"teams"`
	Odds       []actionOdds                 `json:"odds"`    // v1 shape (legacy)
	Markets    map[string]actionBookMarkets `json:"markets"` // v2 shape
}

type actionTeam struct {
	ID       int    `json:"id"`
	FullName string `json:"full_name"`
	Abbr     string `json:"abbr"`
}

type actionBookMarkets struct {
	Event actionEventMarkets `json:"event"`
}

type actionEventMarkets struct {
	Moneyline []actionOutcome `json:"moneyline"`
	Spread    []actionOutcome `json:"spread"`
	Total     []actionOutcome `json:"total"`
}

type actionOutcome struct {
	Side    string         `json:"side"`
	Odds    *int           `json:"odds"`
	Value   *float64       `json:"value"`
	TeamID  int            `json:"team_id"`
	BetInfo *actionBetInfo `json:"bet_info"`

	// Optional Pro fields when present on outcomes.
	EdgePct   *float64 `json:"edge_pct"`
	EdgeGrade string   `json:"edge_grade"`
	Proj      *float64 `json:"proj"`
	ProjOdds  *int     `json:"proj_odds"`
}

type actionBetInfo struct {
	Tickets *actionPctValue `json:"tickets"`
	Money   *actionPctValue `json:"money"`
	Edge    *actionPctValue `json:"edge"`
}

type actionPctValue struct {
	Percent *float64 `json:"percent"`
	Value   *float64 `json:"value"`
	Grade   string   `json:"grade"`
}

// Legacy v1 flat odds fields (kept for fallback / older fixtures).
type actionOdds struct {
	BookID           int      `json:"book_id"`
	MLAway           *int     `json:"ml_away"`
	MLHome           *int     `json:"ml_home"`
	SpreadAway       *float64 `json:"spread_away"`
	SpreadHome       *float64 `json:"spread_home"`
	Total            *float64 `json:"total"`
	MLHomePublic     *float64 `json:"ml_home_public"`
	MLAwayPublic     *float64 `json:"ml_away_public"`
	SpreadHomePublic *float64 `json:"spread_home_public"`
	SpreadAwayPublic *float64 `json:"spread_away_public"`
	TotalUnderPublic *float64 `json:"total_under_public"`
	TotalOverPublic  *float64 `json:"total_over_public"`
	MLHomeMoney      *float64 `json:"ml_home_money"`
	MLAwayMoney      *float64 `json:"ml_away_money"`
	SpreadHomeMoney  *float64 `json:"spread_home_money"`
	SpreadAwayMoney  *float64 `json:"spread_away_money"`
	TotalOverMoney   *float64 `json:"total_over_money"`
	TotalUnderMoney  *float64 `json:"total_under_money"`

	SpreadAwayProj      *float64 `json:"spread_away_proj"`
	SpreadHomeProj      *float64 `json:"spread_home_proj"`
	SpreadAwayEdgePct   *float64 `json:"spread_away_edge_pct"`
	SpreadHomeEdgePct   *float64 `json:"spread_home_edge_pct"`
	SpreadAwayEdgeGrade string   `json:"spread_away_edge_grade"`
	SpreadHomeEdgeGrade string   `json:"spread_home_edge_grade"`
	MLAwayProj          *int     `json:"ml_away_proj"`
	MLHomeProj          *int     `json:"ml_home_proj"`
	MLAwayEdgePct       *float64 `json:"ml_away_edge_pct"`
	MLHomeEdgePct       *float64 `json:"ml_home_edge_pct"`
	MLAwayEdgeGrade     string   `json:"ml_away_edge_grade"`
	MLHomeEdgeGrade     string   `json:"ml_home_edge_grade"`
	OverProj            *float64 `json:"over_proj"`
	UnderProj           *float64 `json:"under_proj"`
	OverEdgePct         *float64 `json:"over_edge_pct"`
	UnderEdgePct        *float64 `json:"under_edge_pct"`
	OverEdgeGrade       string   `json:"over_edge_grade"`
	UnderEdgeGrade      string   `json:"under_edge_grade"`
}

func actionTeams(g actionGame) (away, home actionTeam) {
	byID := map[int]actionTeam{}
	for _, t := range g.Teams {
		byID[t.ID] = t
	}
	if t, ok := byID[g.AwayTeamID]; ok {
		away = t
	}
	if t, ok := byID[g.HomeTeamID]; ok {
		home = t
	}
	if away.FullName == "" && len(g.Teams) > 0 {
		away = g.Teams[0]
	}
	if home.FullName == "" && len(g.Teams) > 1 {
		home = g.Teams[1]
	}
	return away, home
}

func actionMarketsFromGame(away, home actionTeam, g actionGame) []models.MarketSplit {
	if ev, ok := pickActionEventMarkets(g.Markets); ok {
		return actionMarketsFromEvent(away, home, ev)
	}
	return actionMarkets(away, home, pickActionOdds(g.Odds))
}

func pickActionEventMarkets(markets map[string]actionBookMarkets) (actionEventMarkets, bool) {
	if len(markets) == 0 {
		return actionEventMarkets{}, false
	}
	// Prefer book 15 (Action consensus / public-betting board), else richest bet_info.
	bestKey := ""
	bestScore := -1
	if m, ok := markets["15"]; ok {
		if s := actionEventScore(m.Event); s > bestScore {
			bestKey, bestScore = "15", s
		}
	}
	for k, m := range markets {
		if s := actionEventScore(m.Event); s > bestScore || (s == bestScore && bestKey != "15" && k == "15") {
			bestKey, bestScore = k, s
		}
	}
	if bestKey == "" || bestScore <= 0 {
		// Still return a book if present so lines/odds can show even without splits.
		if m, ok := markets["15"]; ok {
			return m.Event, true
		}
		for _, m := range markets {
			return m.Event, true
		}
		return actionEventMarkets{}, false
	}
	return markets[bestKey].Event, true
}

func actionEventScore(ev actionEventMarkets) int {
	n := 0
	for _, o := range append(append(ev.Spread, ev.Total...), ev.Moneyline...) {
		if pct := outcomeBetPct(o); pct != nil {
			n++
		}
		if pct := outcomeMoneyPct(o); pct != nil {
			n++
		}
	}
	return n
}

func actionMarketsFromEvent(away, home actionTeam, ev actionEventMarkets) []models.MarketSplit {
	return []models.MarketSplit{
		{
			Market: models.MarketSpread,
			Sides: []models.SideSplit{
				outcomeSide(away.FullName, models.SideAway, findOutcome(ev.Spread, "away")),
				outcomeSide(home.FullName, models.SideHome, findOutcome(ev.Spread, "home")),
			},
		},
		{
			Market: models.MarketTotal,
			Sides: []models.SideSplit{
				outcomeSide("Over", models.SideOver, findOutcome(ev.Total, "over")),
				outcomeSide("Under", models.SideUnder, findOutcome(ev.Total, "under")),
			},
		},
		{
			Market: models.MarketMoneyline,
			Sides: []models.SideSplit{
				outcomeSide(away.FullName, models.SideAway, findOutcome(ev.Moneyline, "away")),
				outcomeSide(home.FullName, models.SideHome, findOutcome(ev.Moneyline, "home")),
			},
		},
	}
}

func findOutcome(list []actionOutcome, side string) *actionOutcome {
	side = strings.ToLower(side)
	for i := range list {
		if strings.ToLower(list[i].Side) == side {
			return &list[i]
		}
	}
	return nil
}

func outcomeSide(label string, side models.Side, o *actionOutcome) models.SideSplit {
	ss := models.SideSplit{Label: label, Side: side}
	if o == nil {
		return ss
	}
	ss.Odds = o.Odds
	if side == models.SideOver || side == models.SideUnder || side == models.SideAway || side == models.SideHome {
		ss.Line = o.Value
		// Moneyline value is often 0; omit useless line.
		if side == models.SideAway || side == models.SideHome {
			if o.Value != nil && *o.Value == 0 && o.Odds != nil {
				ss.Line = nil
			}
		}
	}
	ss.BetPct = outcomeBetPct(*o)
	ss.MoneyPct = outcomeMoneyPct(*o)
	return ss
}

func outcomeBetPct(o actionOutcome) *float64 {
	if o.BetInfo != nil && o.BetInfo.Tickets != nil {
		return o.BetInfo.Tickets.Percent
	}
	return nil
}

func outcomeMoneyPct(o actionOutcome) *float64 {
	if o.BetInfo != nil && o.BetInfo.Money != nil {
		return o.BetInfo.Money.Percent
	}
	return nil
}

func actionProInsightsFromGame(away, home actionTeam, g actionGame) []models.ProInsight {
	ev, ok := pickActionEventMarkets(g.Markets)
	if !ok {
		return nil
	}
	var out []models.ProInsight
	if insight, ok := pickMarketInsight(
		models.MarketSpread,
		outcomeInsight(models.SideAway, formatSpreadLabel(away.FullName, nil, findOutcome(ev.Spread, "away")), findOutcome(ev.Spread, "away")),
		outcomeInsight(models.SideHome, formatSpreadLabel(home.FullName, nil, findOutcome(ev.Spread, "home")), findOutcome(ev.Spread, "home")),
	); ok {
		out = append(out, insight)
	}
	if insight, ok := pickMarketInsight(
		models.MarketTotal,
		outcomeInsight(models.SideOver, formatTotalLabel("Over", nil, findOutcome(ev.Total, "over")), findOutcome(ev.Total, "over")),
		outcomeInsight(models.SideUnder, formatTotalLabel("Under", nil, findOutcome(ev.Total, "under")), findOutcome(ev.Total, "under")),
	); ok {
		out = append(out, insight)
	}
	if insight, ok := pickMarketInsight(
		models.MarketMoneyline,
		outcomeInsight(models.SideAway, away.FullName, findOutcome(ev.Moneyline, "away")),
		outcomeInsight(models.SideHome, home.FullName, findOutcome(ev.Moneyline, "home")),
	); ok {
		out = append(out, insight)
	}
	return out
}

func outcomeInsight(side models.Side, label string, o *actionOutcome) candidateInsight {
	c := candidateInsight{side: side, label: label}
	if o == nil {
		return c
	}
	c.edge = o.EdgePct
	c.grade = o.EdgeGrade
	c.odds = o.ProjOdds
	if o.BetInfo != nil && o.BetInfo.Edge != nil {
		if c.edge == nil {
			c.edge = o.BetInfo.Edge.Percent
		}
		if c.grade == "" {
			c.grade = o.BetInfo.Edge.Grade
		}
	}
	return c
}

func formatSpreadLabel(team string, proj *float64, o *actionOutcome) string {
	var line *float64
	if o != nil {
		line = o.Value
		if o.Proj != nil {
			proj = o.Proj
		}
	}
	return formatSpreadLabelValues(team, proj, line)
}

func formatTotalLabel(side string, proj *float64, o *actionOutcome) string {
	var line *float64
	if o != nil {
		line = o.Value
		if o.Proj != nil {
			proj = o.Proj
		}
	}
	return formatTotalLabelValues(side, proj, line)
}

func pickActionOdds(odds []actionOdds) *actionOdds {
	if len(odds) == 0 {
		return nil
	}
	best := &odds[0]
	bestScore := actionOddsScore(odds[0])
	for i := range odds[1:] {
		o := &odds[i+1]
		if s := actionOddsScore(*o); s > bestScore {
			best = o
			bestScore = s
		}
	}
	return best
}

func actionOddsScore(o actionOdds) int {
	vals := []*float64{
		o.MLHomePublic, o.MLAwayPublic, o.SpreadHomePublic, o.SpreadAwayPublic,
		o.TotalUnderPublic, o.TotalOverPublic, o.MLHomeMoney, o.MLAwayMoney,
		o.SpreadHomeMoney, o.SpreadAwayMoney, o.TotalOverMoney, o.TotalUnderMoney,
		o.SpreadAwayEdgePct, o.SpreadHomeEdgePct, o.MLAwayEdgePct, o.MLHomeEdgePct,
		o.OverEdgePct, o.UnderEdgePct,
	}
	n := 0
	for _, v := range vals {
		if v != nil {
			n++
		}
	}
	for _, g := range []string{o.SpreadAwayEdgeGrade, o.SpreadHomeEdgeGrade, o.MLAwayEdgeGrade, o.MLHomeEdgeGrade, o.OverEdgeGrade, o.UnderEdgeGrade} {
		if g != "" {
			n++
		}
	}
	return n
}

func actionMarkets(away, home actionTeam, o *actionOdds) []models.MarketSplit {
	if o == nil {
		return nil
	}
	return []models.MarketSplit{
		{
			Market: models.MarketSpread,
			Sides: []models.SideSplit{
				{Label: away.FullName, Side: models.SideAway, Line: o.SpreadAway, BetPct: o.SpreadAwayPublic, MoneyPct: o.SpreadAwayMoney},
				{Label: home.FullName, Side: models.SideHome, Line: o.SpreadHome, BetPct: o.SpreadHomePublic, MoneyPct: o.SpreadHomeMoney},
			},
		},
		{
			Market: models.MarketTotal,
			Sides: []models.SideSplit{
				{Label: "Over", Side: models.SideOver, Line: o.Total, BetPct: o.TotalOverPublic, MoneyPct: o.TotalOverMoney},
				{Label: "Under", Side: models.SideUnder, Line: o.Total, BetPct: o.TotalUnderPublic, MoneyPct: o.TotalUnderMoney},
			},
		},
		{
			Market: models.MarketMoneyline,
			Sides: []models.SideSplit{
				{Label: away.FullName, Side: models.SideAway, Odds: o.MLAway, BetPct: o.MLAwayPublic, MoneyPct: o.MLAwayMoney},
				{Label: home.FullName, Side: models.SideHome, Odds: o.MLHome, BetPct: o.MLHomePublic, MoneyPct: o.MLHomeMoney},
			},
		},
	}
}

func actionProInsights(away, home actionTeam, o *actionOdds) []models.ProInsight {
	if o == nil {
		return nil
	}
	var out []models.ProInsight
	if insight, ok := pickMarketInsight(
		models.MarketSpread,
		candidateInsight{side: models.SideAway, label: formatSpreadLabelValues(away.FullName, o.SpreadAwayProj, o.SpreadAway), grade: o.SpreadAwayEdgeGrade, edge: o.SpreadAwayEdgePct},
		candidateInsight{side: models.SideHome, label: formatSpreadLabelValues(home.FullName, o.SpreadHomeProj, o.SpreadHome), grade: o.SpreadHomeEdgeGrade, edge: o.SpreadHomeEdgePct},
	); ok {
		out = append(out, insight)
	}
	if insight, ok := pickMarketInsight(
		models.MarketTotal,
		candidateInsight{side: models.SideOver, label: formatTotalLabelValues("Over", o.OverProj, o.Total), grade: o.OverEdgeGrade, edge: o.OverEdgePct},
		candidateInsight{side: models.SideUnder, label: formatTotalLabelValues("Under", o.UnderProj, o.Total), grade: o.UnderEdgeGrade, edge: o.UnderEdgePct},
	); ok {
		out = append(out, insight)
	}
	if insight, ok := pickMarketInsight(
		models.MarketMoneyline,
		candidateInsight{side: models.SideAway, label: away.FullName, grade: o.MLAwayEdgeGrade, edge: o.MLAwayEdgePct, odds: o.MLAwayProj},
		candidateInsight{side: models.SideHome, label: home.FullName, grade: o.MLHomeEdgeGrade, edge: o.MLHomeEdgePct, odds: o.MLHomeProj},
	); ok {
		out = append(out, insight)
	}
	return out
}

type candidateInsight struct {
	side  models.Side
	label string
	grade string
	edge  *float64
	odds  *int
}

func pickMarketInsight(market models.Market, a, b candidateInsight) (models.ProInsight, bool) {
	aOK := a.grade != "" || a.edge != nil
	bOK := b.grade != "" || b.edge != nil
	if !aOK && !bOK {
		return models.ProInsight{}, false
	}
	pick := a
	if !aOK {
		pick = b
	} else if bOK {
		ae, be := -1e9, -1e9
		if a.edge != nil {
			ae = *a.edge
		}
		if b.edge != nil {
			be = *b.edge
		}
		if be > ae {
			pick = b
		}
	}
	return models.ProInsight{
		Market:   market,
		Side:     pick.side,
		Label:    pick.label,
		Grade:    pick.grade,
		EdgePct:  pick.edge,
		ProjOdds: pick.odds,
	}, true
}

func formatSpreadLabelValues(team string, proj, line *float64) string {
	v := proj
	if v == nil {
		v = line
	}
	if v == nil {
		return team
	}
	n := *v
	if n > 0 {
		return fmt.Sprintf("%s +%g", team, n)
	}
	return fmt.Sprintf("%s %g", team, n)
}

func formatTotalLabelValues(side string, proj, line *float64) string {
	v := proj
	if v == nil {
		v = line
	}
	if v == nil {
		return side
	}
	prefix := "O"
	if side == "Under" {
		prefix = "U"
	}
	return fmt.Sprintf("%s %g", prefix, *v)
}

func hasAnySplit(g models.GameSplits) bool {
	for _, m := range g.Markets {
		for _, s := range m.Sides {
			if s.BetPct != nil || s.MoneyPct != nil {
				return true
			}
		}
	}
	return false
}
