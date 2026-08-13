package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"nfl-data-scraper/internal/httpx"
	"nfl-data-scraper/internal/models"
)

// ActionNetwork collects public betting percentages from Action Network's
// scoreboard API when the provider exposes bet/money split fields.
type ActionNetwork struct {
	client *httpx.Client
}

func NewActionNetwork() *ActionNetwork {
	c := httpx.New(45 * time.Second)
	c.Referer = "https://www.actionnetwork.com/nfl/public-betting"
	return &ActionNetwork{client: c}
}

func (a *ActionNetwork) ID() string   { return "action-network" }
func (a *ActionNetwork) Name() string { return "Action Network" }

const actionPublicBettingURL = "https://api.actionnetwork.com/web/v1/scoreboard/publicbetting/nfl"

func (a *ActionNetwork) Collect(ctx context.Context) ([]models.GameSplits, error) {
	body, _, err := a.client.Get(ctx, actionPublicBettingURL)
	if err != nil {
		return nil, err
	}

	var payload actionScoreboard
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode action network json: %w", err)
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
			SourceID:   a.ID(),
			SourceName: a.Name(),
			Book:       "Action Network Consensus",
			ExternalID: fmt.Sprintf("%d", g.ID),
			AwayTeam:   away.FullName,
			HomeTeam:   home.FullName,
			AwayAbbr:   away.Abbr,
			HomeAbbr:   home.Abbr,
			Season:     g.Season,
			Week:       g.Week,
			SeasonType: g.Type,
			FetchedAt:  now,
			URL:        "https://www.actionnetwork.com/nfl/public-betting",
			Markets:    actionMarkets(away, home, odds),
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
		return nil, fmt.Errorf("action network NFL scoreboard reachable (%d games) but bet/money split fields are empty (often paywalled or unavailable in preseason)", len(payload.Games))
	}
	return out, nil
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
	// Prefer the entry with the most populated public/money fields.
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
	}
	n := 0
	for _, v := range vals {
		if v != nil {
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
