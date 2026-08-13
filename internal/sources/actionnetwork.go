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
// When ACTION_NETWORK_COOKIE is provided, authenticated Pro fields unlock
// and game projections (grade/edge) are joined onto each game.
type ActionNetwork struct {
	client *httpx.Client
	cookie string
}

func NewActionNetwork(cookie string) *ActionNetwork {
	c := httpx.New(45 * time.Second)
	c.Referer = "https://www.actionnetwork.com/nfl/public-betting"
	c.Cookie = strings.TrimSpace(cookie)
	return &ActionNetwork{client: c, cookie: c.Cookie}
}

func (a *ActionNetwork) ID() string   { return "action-network" }
func (a *ActionNetwork) Name() string { return "Action Network" }

const (
	actionPublicBettingURL    = "https://api.actionnetwork.com/web/v1/scoreboard/publicbetting/nfl"
	actionGameProjectionsURL  = "https://api.actionnetwork.com/web/v1/scoreboard/gameprojections/nfl"
	actionPublicBettingPage   = "https://www.actionnetwork.com/nfl/public-betting"
	actionProjectionsPage     = "https://www.actionnetwork.com/nfl/projections"
	actionAuthExpiredMessage  = "Action Network auth failed or cookie expired — refresh ACTION_NETWORK_COOKIE"
)

func (a *ActionNetwork) Collect(ctx context.Context) ([]models.GameSplits, error) {
	body, _, err := a.client.Get(ctx, actionPublicBettingURL)
	if err != nil {
		if a.cookie != "" && isHTTPAuthError(err) {
			return nil, fmt.Errorf("%s", actionAuthExpiredMessage)
		}
		return nil, err
	}

	var payload actionScoreboard
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode action network json: %w", err)
	}

	insightsByGame := map[int][]models.ProInsight{}
	if a.cookie != "" {
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
		odds := pickActionOdds(g.Odds)
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
			Markets:     actionMarkets(away, home, odds),
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
		if a.cookie != "" {
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
		odds := pickActionOdds(g.Odds)
		insights := actionProInsights(away, home, odds)
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
	ID          int          `json:"id"`
	StartTime   string       `json:"start_time"`
	AwayTeamID  int          `json:"away_team_id"`
	HomeTeamID  int          `json:"home_team_id"`
	Season      int          `json:"season"`
	Week        int          `json:"week"`
	Type        string       `json:"type"`
	NumBets     int          `json:"num_bets"`
	Teams       []actionTeam `json:"teams"`
	Odds        []actionOdds `json:"odds"`
}

type actionTeam struct {
	ID       int    `json:"id"`
	FullName string `json:"full_name"`
	Abbr     string `json:"abbr"`
}

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

	// Pro projection / edge fields (present when authenticated).
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

func pickActionOdds(odds []actionOdds) *actionOdds {
	if len(odds) == 0 {
		return nil
	}
	// Prefer the entry with the most populated public/money/pro fields.
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
		candidateInsight{side: models.SideAway, label: formatSpreadLabel(away.FullName, o.SpreadAwayProj, o.SpreadAway), grade: o.SpreadAwayEdgeGrade, edge: o.SpreadAwayEdgePct},
		candidateInsight{side: models.SideHome, label: formatSpreadLabel(home.FullName, o.SpreadHomeProj, o.SpreadHome), grade: o.SpreadHomeEdgeGrade, edge: o.SpreadHomeEdgePct},
	); ok {
		out = append(out, insight)
	}
	if insight, ok := pickMarketInsight(
		models.MarketTotal,
		candidateInsight{side: models.SideOver, label: formatTotalLabel("Over", o.OverProj, o.Total), grade: o.OverEdgeGrade, edge: o.OverEdgePct},
		candidateInsight{side: models.SideUnder, label: formatTotalLabel("Under", o.UnderProj, o.Total), grade: o.UnderEdgeGrade, edge: o.UnderEdgePct},
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

func formatSpreadLabel(team string, proj, line *float64) string {
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

func formatTotalLabel(side string, proj, line *float64) string {
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
