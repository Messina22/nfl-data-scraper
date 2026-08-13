package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"nfl-data-scraper/internal/httpx"
	"nfl-data-scraper/internal/models"
)

// CoversConsensus scrapes Covers contest consensus / public-money pages.
// These figures reflect Covers contestant picks (reported consensus), which
// is a different population than sportsbook handle, but is a commonly cited source.
type CoversConsensus struct {
	client *httpx.Client
}

func NewCoversConsensus() *CoversConsensus {
	return &CoversConsensus{client: httpx.New(45 * time.Second)}
}

func (c *CoversConsensus) ID() string   { return "covers-consensus" }
func (c *CoversConsensus) Name() string { return "Covers Consensus" }

const coversNFLURL = "https://contests.covers.com/consensus/topconsensus/nfl/overall"

func (c *CoversConsensus) Collect(ctx context.Context) ([]models.GameSplits, error) {
	_ = ctx
	body, finalURL, err := c.client.Get(coversNFLURL)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse covers html: %w", err)
	}

	now := time.Now().UTC()
	var out []models.GameSplits

	doc.Find(".covers-CoversConsensusDetails-gameBox, .covers-CoversConsensus-gameBox, .cmg_game_data").Each(func(_ int, s *goquery.Selection) {
		if g, ok := parseCoversGameBox(s, now, finalURL, c.ID(), c.Name()); ok {
			out = append(out, g)
		}
	})

	if len(out) == 0 {
		return nil, fmt.Errorf("covers NFL consensus currently has no matchup split rows (common in off/preseason); source is wired for when contests populate")
	}
	return out, nil
}

func parseCoversGameBox(s *goquery.Selection, now time.Time, pageURL, id, name string) (models.GameSplits, bool) {
	teams := s.Find(".covers-CoversConsensusDetails-teamName, .team-name, .covers-CoversConsensus-team")
	pcts := []float64{}
	s.Find("*").Each(func(_ int, n *goquery.Selection) {
		t := strings.TrimSpace(n.Text())
		if strings.HasSuffix(t, "%") && len(t) <= 4 {
			if p := parsePct(t); p != nil {
				pcts = append(pcts, *p)
			}
		}
	})
	if teams.Length() < 2 || len(pcts) < 2 {
		return models.GameSplits{}, false
	}
	away := strings.TrimSpace(teams.Eq(0).Text())
	home := strings.TrimSpace(teams.Eq(1).Text())
	if away == "" || home == "" {
		return models.GameSplits{}, false
	}
	awayPct, homePct := pcts[0], pcts[1]
	return models.GameSplits{
		SourceID:   id,
		SourceName: name,
		Book:       "Covers Contest Consensus",
		AwayTeam:   away,
		HomeTeam:   home,
		FetchedAt:  now,
		URL:        pageURL,
		Markets: []models.MarketSplit{{
			Market: models.MarketSpread,
			Sides: []models.SideSplit{
				{Label: away, Side: models.SideAway, BetPct: &awayPct},
				{Label: home, Side: models.SideHome, BetPct: &homePct},
			},
		}},
	}, true
}
