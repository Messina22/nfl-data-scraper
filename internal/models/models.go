package models

import "time"

// Market identifies a wager type reported by a source.
type Market string

const (
	MarketSpread    Market = "spread"
	MarketMoneyline Market = "moneyline"
	MarketTotal     Market = "total"
)

// Side identifies which outcome the percentages refer to.
// For totals, Away is Over and Home is Under by convention in this project
// when reporting paired sides; SideSplit carries explicit labels.
type Side string

const (
	SideAway Side = "away"
	SideHome Side = "home"
	SideOver Side = "over"
	SideUnder Side = "under"
)

// SideSplit is the bet/money (handle) share for one outcome.
type SideSplit struct {
	Label      string   `json:"label"`
	Side       Side     `json:"side"`
	Line       *float64 `json:"line,omitempty"`
	Odds       *int     `json:"odds,omitempty"` // American odds when available
	BetPct     *float64 `json:"bet_pct,omitempty"`
	MoneyPct   *float64 `json:"money_pct,omitempty"` // also called handle %
}

// MarketSplit is the full reported split for one market on a game.
type MarketSplit struct {
	Market Market      `json:"market"`
	Sides  []SideSplit `json:"sides"`
}

// GameSplits is one game's reported splits from a single source/book.
type GameSplits struct {
	SourceID   string         `json:"source_id"`
	SourceName string         `json:"source_name"`
	Book       string         `json:"book,omitempty"`
	ExternalID string         `json:"external_id,omitempty"`
	StartTime  *time.Time     `json:"start_time,omitempty"`
	AwayTeam   string         `json:"away_team"`
	HomeTeam   string         `json:"home_team"`
	AwayAbbr   string         `json:"away_abbr,omitempty"`
	HomeAbbr   string         `json:"home_abbr,omitempty"`
	Season     int            `json:"season,omitempty"`
	Week       int            `json:"week,omitempty"`
	SeasonType string         `json:"season_type,omitempty"`
	NumBets    *int           `json:"num_bets,omitempty"`
	Markets    []MarketSplit  `json:"markets"`
	FetchedAt  time.Time      `json:"fetched_at"`
	URL        string         `json:"url,omitempty"`
}

// SourceStatus reports what happened when a source was collected.
type SourceStatus struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OK        bool      `json:"ok"`
	Games     int       `json:"games"`
	Error     string    `json:"error,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	URL       string    `json:"url,omitempty"`
}

// Snapshot is a point-in-time collection across all sources.
type Snapshot struct {
	CollectedAt time.Time      `json:"collected_at"`
	Sources     []SourceStatus `json:"sources"`
	Games       []GameSplits   `json:"games"`
}
