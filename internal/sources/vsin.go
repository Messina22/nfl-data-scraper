package sources

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"nfl-data-scraper/internal/httpx"
	"nfl-data-scraper/internal/models"
)

// VSiN scrapes published sportsbook betting splits (handle % + bets %)
// for spread, total, and moneyline from data.vsin.com.
type VSiN struct {
	BookCode string
	BookName string
	client   *httpx.Client
}

func NewVSiN(bookCode, bookName string) *VSiN {
	return &VSiN{
		BookCode: bookCode,
		BookName: bookName,
		client:   httpx.New(45 * time.Second),
	}
}

func (v *VSiN) ID() string {
	return "vsin-" + strings.ToLower(v.BookCode)
}

func (v *VSiN) Name() string {
	return "VSiN (" + v.BookName + ")"
}

func (v *VSiN) pageURL() string {
	q := url.Values{}
	q.Set("source", v.BookCode)
	q.Set("sport", "NFL")
	q.Set("display", "table")
	return "https://data.vsin.com/betting-splits/?" + q.Encode()
}

func (v *VSiN) Collect(ctx context.Context) ([]models.GameSplits, error) {
	_ = ctx
	page := v.pageURL()
	body, finalURL, err := v.client.Get(page)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	now := time.Now().UTC()
	var out []models.GameSplits
	doc.Find("table.sp-table tbody").Each(func(_ int, tbody *goquery.Selection) {
		var pending *vsinRow
		tbody.Find("tr.sp-row").Each(func(_ int, row *goquery.Selection) {
			parsed := parseVSiNRow(row)
			if parsed.team == "" {
				return
			}
			if pending == nil {
				pending = &parsed
				return
			}
			away, home := *pending, parsed
			pending = nil
			if away.gameCode != "" && home.gameCode != "" && away.gameCode != home.gameCode {
				// Misaligned row; start a new pair from the current row.
				pending = &home
				return
			}
			gameCode := away.gameCode
			if gameCode == "" {
				gameCode = home.gameCode
			}
			gs := models.GameSplits{
				SourceID:   v.ID(),
				SourceName: v.Name(),
				Book:       v.BookName,
				ExternalID: gameCode,
				AwayTeam:   away.team,
				HomeTeam:   home.team,
				FetchedAt:  now,
				URL:        finalURL,
				Markets: []models.MarketSplit{
					{
						Market: models.MarketSpread,
						Sides: []models.SideSplit{
							sideFrom(away.team, models.SideAway, away.spreadLine, away.spreadHandle, away.spreadBets, nil),
							sideFrom(home.team, models.SideHome, home.spreadLine, home.spreadHandle, home.spreadBets, nil),
						},
					},
					{
						Market: models.MarketTotal,
						Sides: []models.SideSplit{
							sideFrom("Over", models.SideOver, away.totalLine, away.totalHandle, away.totalBets, nil),
							sideFrom("Under", models.SideUnder, home.totalLine, home.totalHandle, home.totalBets, nil),
						},
					},
					{
						Market: models.MarketMoneyline,
						Sides: []models.SideSplit{
							sideFrom(away.team, models.SideAway, nil, away.mlHandle, away.mlBets, away.mlOdds),
							sideFrom(home.team, models.SideHome, nil, home.mlHandle, home.mlBets, home.mlOdds),
						},
					},
				},
			}
			if t := parseVSiNGameTime(gameCode); t != nil {
				gs.StartTime = t
			}
			out = append(out, gs)
		})
	})

	if len(out) == 0 {
		return nil, fmt.Errorf("no VSiN NFL splits rows found for book %s (page may be empty or markup changed)", v.BookCode)
	}
	return out, nil
}

type vsinRow struct {
	gameCode     string
	team         string
	spreadLine   *float64
	spreadHandle *float64
	spreadBets   *float64
	totalLine    *float64
	totalHandle  *float64
	totalBets    *float64
	mlOdds       *int
	mlHandle     *float64
	mlBets       *float64
}

func parseVSiNRow(sel *goquery.Selection) vsinRow {
	var r vsinRow
	if btn := sel.Find("button.sp-act-history"); btn.Length() > 0 {
		r.gameCode, _ = btn.Attr("data-gamecode")
	}
	r.team = strings.TrimSpace(sel.Find("td.sp-cell-team").Text())
	cells := sel.Find("td")
	// Expected order:
	// 0 action, 1 team, 2 spread line, 3 spread handle, 4 spread bets,
	// 5 total line, 6 total handle, 7 total bets, 8 ml odds, 9 ml handle, 10 ml bets
	get := func(i int) string {
		if i >= cells.Length() {
			return ""
		}
		return strings.TrimSpace(cells.Eq(i).Text())
	}
	r.spreadLine = parseLine(get(2))
	r.spreadHandle = parsePct(get(3))
	r.spreadBets = parsePct(get(4))
	r.totalLine = parseLine(get(5))
	r.totalHandle = parsePct(get(6))
	r.totalBets = parsePct(get(7))
	r.mlOdds = parseAmericanOdds(get(8))
	r.mlHandle = parsePct(get(9))
	r.mlBets = parsePct(get(10))
	return r
}

func sideFrom(label string, side models.Side, line, money, bets *float64, odds *int) models.SideSplit {
	return models.SideSplit{
		Label:    label,
		Side:     side,
		Line:     line,
		Odds:     odds,
		BetPct:   bets,
		MoneyPct: money,
	}
}

func parsePct(s string) *float64 {
	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	if s == "" || s == "-" || s == "—" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

func parseLine(s string) *float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, "½", ".5"))
	s = strings.TrimSpace(strings.ReplaceAll(s, "pk", "0"))
	s = strings.TrimSpace(strings.ReplaceAll(s, "PK", "0"))
	if s == "" || s == "-" || s == "—" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

func parseAmericanOdds(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" || s == "—" {
		return nil
	}
	s = strings.ReplaceAll(s, "−", "-")
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}

// game codes look like 20260909NFL00061 → 2026-09-09
func parseVSiNGameTime(code string) *time.Time {
	if len(code) < 8 {
		return nil
	}
	t, err := time.Parse("20060102", code[:8])
	if err != nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}
