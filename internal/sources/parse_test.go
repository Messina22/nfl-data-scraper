package sources

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"

	"nfl-data-scraper/internal/models"
)

func TestParsePctAndLine(t *testing.T) {
	if got := parsePct("56%"); got == nil || *got != 56 {
		t.Fatalf("parsePct 56%% = %v", got)
	}
	if got := parseLine("-3.5"); got == nil || *got != -3.5 {
		t.Fatalf("parseLine -3.5 = %v", got)
	}
	if got := parseAmericanOdds("+160"); got == nil || *got != 160 {
		t.Fatalf("odds +160 = %v", got)
	}
	if got := parseAmericanOdds("-192"); got == nil || *got != -192 {
		t.Fatalf("odds -192 = %v", got)
	}
}

func TestParseVSiNGameTime(t *testing.T) {
	tm := parseVSiNGameTime("20260909NFL00061")
	if tm == nil || tm.Format("2006-01-02") != "2026-09-09" {
		t.Fatalf("unexpected time %v", tm)
	}
}

func TestVSiNPageURL(t *testing.T) {
	v := NewVSiN("DK", "DraftKings")
	u := v.pageURL()
	if !strings.Contains(u, "source=DK") || !strings.Contains(u, "sport=NFL") {
		t.Fatalf("bad url %s", u)
	}
}

func TestParseCoversGameBoxNestedPercents(t *testing.T) {
	// Nested % markup previously yielded [62,62,38,38] via ancestor .Text().
	html := `
	<div class="covers-CoversConsensusDetails-gameBox">
	  <div class="covers-CoversConsensusDetails-teamName">Away FC</div>
	  <div class="covers-CoversConsensusDetails-teamName">Home United</div>
	  <div class="pct"><span>62%</span></div>
	  <div class="pct"><span>38%</span></div>
	</div>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	box := doc.Find(".covers-CoversConsensusDetails-gameBox")
	g, ok := parseCoversGameBox(box, time.Now().UTC(), "http://example", "covers-consensus", "Covers")
	if !ok {
		t.Fatal("expected parse ok")
	}
	if len(g.Markets) != 1 || len(g.Markets[0].Sides) != 2 {
		t.Fatalf("bad markets: %+v", g.Markets)
	}
	away := *g.Markets[0].Sides[0].BetPct
	home := *g.Markets[0].Sides[1].BetPct
	if away != 62 || home != 38 {
		t.Fatalf("got %v/%v, want 62/38", away, home)
	}
}

func TestParseCoversGameBoxRejectsDuplicateSides(t *testing.T) {
	html := `
	<div class="covers-CoversConsensusDetails-gameBox">
	  <div class="covers-CoversConsensusDetails-teamName">Away FC</div>
	  <div class="covers-CoversConsensusDetails-teamName">Home United</div>
	  <span>62%</span>
	  <span>62%</span>
	</div>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	_, ok := parseCoversGameBox(doc.Find(".covers-CoversConsensusDetails-gameBox"), time.Now().UTC(), "", "covers-consensus", "Covers")
	if ok {
		t.Fatal("expected reject when sides do not sum near 100")
	}
}

func TestActionProInsightsFromOdds(t *testing.T) {
	away := actionTeam{FullName: "Green Bay Packers", Abbr: "GB"}
	home := actionTeam{FullName: "Pittsburgh Steelers", Abbr: "PIT"}
	spreadAway := -3.5
	spreadHome := 3.5
	awayEdge := 4.2
	homeEdge := -1.1
	over := 44.5
	overEdge := 2.0
	mlAway := -120
	mlHome := 100
	mlAwayEdge := 1.5
	mlHomeEdge := -0.5
	o := &actionOdds{
		SpreadAway:          &spreadAway,
		SpreadHome:          &spreadHome,
		SpreadAwayProj:      &spreadAway,
		SpreadHomeProj:      &spreadHome,
		SpreadAwayEdgePct:   &awayEdge,
		SpreadHomeEdgePct:   &homeEdge,
		SpreadAwayEdgeGrade: "B+",
		SpreadHomeEdgeGrade: "D",
		Total:               &over,
		OverProj:            &over,
		UnderProj:           &over,
		OverEdgePct:         &overEdge,
		OverEdgeGrade:       "B",
		MLAwayProj:          &mlAway,
		MLHomeProj:          &mlHome,
		MLAwayEdgePct:       &mlAwayEdge,
		MLHomeEdgePct:       &mlHomeEdge,
		MLAwayEdgeGrade:     "C+",
		MLHomeEdgeGrade:     "C",
	}

	insights := actionProInsights(away, home, o)
	if len(insights) != 3 {
		t.Fatalf("expected 3 market insights, got %d: %+v", len(insights), insights)
	}

	byMarket := map[string]models.ProInsight{}
	for _, in := range insights {
		byMarket[string(in.Market)] = in
	}
	spread := byMarket["spread"]
	if spread.Side != models.SideAway || spread.Grade != "B+" || spread.EdgePct == nil || *spread.EdgePct != 4.2 {
		t.Fatalf("spread insight = %+v", spread)
	}
	if !strings.Contains(spread.Label, "Packers") {
		t.Fatalf("spread label = %q", spread.Label)
	}
	total := byMarket["total"]
	if total.Side != models.SideOver || total.Grade != "B" {
		t.Fatalf("total insight = %+v", total)
	}
	ml := byMarket["moneyline"]
	if ml.Side != models.SideAway || ml.ProjOdds == nil || *ml.ProjOdds != -120 {
		t.Fatalf("moneyline insight = %+v", ml)
	}
}

func TestActionProInsightsEmptyWithoutProFields(t *testing.T) {
	away := actionTeam{FullName: "A", Abbr: "A"}
	home := actionTeam{FullName: "B", Abbr: "B"}
	o := &actionOdds{MLAway: intPtr(-110), MLHome: intPtr(-110)}
	if got := actionProInsights(away, home, o); len(got) != 0 {
		t.Fatalf("expected no insights, got %+v", got)
	}
}

func TestActionMarketsFromV2Markets(t *testing.T) {
	raw := []byte(`{
	  "id": 1,
	  "away_team_id": 147,
	  "home_team_id": 132,
	  "teams": [
	    {"id": 132, "full_name": "Pittsburgh Steelers", "abbr": "PIT"},
	    {"id": 147, "full_name": "Green Bay Packers", "abbr": "GB"}
	  ],
	  "markets": {
	    "15": {
	      "event": {
	        "spread": [
	          {"side":"away","value":-3,"odds":-105,"bet_info":{"tickets":{"percent":55},"money":{"percent":66}}},
	          {"side":"home","value":3,"odds":-115,"bet_info":{"tickets":{"percent":45},"money":{"percent":34}}}
	        ],
	        "total": [
	          {"side":"over","value":38.5,"odds":-110,"bet_info":{"tickets":{"percent":76},"money":{"percent":76}}},
	          {"side":"under","value":38.5,"odds":-108,"bet_info":{"tickets":{"percent":24},"money":{"percent":24}}}
	        ],
	        "moneyline": [
	          {"side":"away","value":0,"odds":-162,"bet_info":{"tickets":{"percent":55},"money":{"percent":55}}},
	          {"side":"home","value":0,"odds":136,"bet_info":{"tickets":{"percent":45},"money":{"percent":45}}}
	        ]
	      }
	    }
	  }
	}`)
	var g actionGame
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	away, home := actionTeams(g)
	markets := actionMarketsFromGame(away, home, g)
	if len(markets) != 3 {
		t.Fatalf("markets=%d", len(markets))
	}
	gs := models.GameSplits{Markets: markets}
	if !hasAnySplit(gs) {
		t.Fatal("expected splits from v2 markets")
	}
	spread := markets[0]
	if spread.Sides[0].BetPct == nil || *spread.Sides[0].BetPct != 55 {
		t.Fatalf("spread away bet=%v", spread.Sides[0].BetPct)
	}
	if spread.Sides[0].MoneyPct == nil || *spread.Sides[0].MoneyPct != 66 {
		t.Fatalf("spread away money=%v", spread.Sides[0].MoneyPct)
	}
	if spread.Sides[0].Line == nil || *spread.Sides[0].Line != -3 {
		t.Fatalf("spread away line=%v", spread.Sides[0].Line)
	}
}

func intPtr(v int) *int { return &v }

func TestDKNetworkPageURL(t *testing.T) {
	u := dkNetworkPageURL("88808", "NFL", 1)
	if !strings.Contains(u, "tb_edate=n30days") || !strings.Contains(u, "tb_emt=0") {
		t.Fatalf("expected n30days + all markets, got %s", u)
	}
	if strings.Contains(u, "tb_eg=MLB") && !strings.Contains(u, "88808") {
		t.Fatalf("NFL url should not be MLB-only: %s", u)
	}
	if !strings.Contains(u, "tb_eg=88808") {
		t.Fatalf("missing event group: %s", u)
	}
	if strings.Contains(u, "tb_page=") {
		t.Fatalf("page 1 should omit tb_page: %s", u)
	}
	p2 := dkNetworkPageURL("MLB", "MLB", 2)
	if !strings.Contains(p2, "tb_page=2") || !strings.Contains(p2, "tb_eg=MLB") {
		t.Fatalf("page 2 url = %s", p2)
	}
	mlb := dkNetworkPageURL("84240", "MLB", 1)
	nfl := dkNetworkPageURL("88808", "NFL", 1)
	if mlb == nfl {
		t.Fatal("MLB and NFL urls should differ")
	}
}

func TestParseDKNetworkSportsSlugAndNumeric(t *testing.T) {
	html := `
	<select name="tb_eg" id="sid-list">
	  <option value="Sports">Sports</option>
	  <option value="0">Sports</option>
	  <option value="NFL">NFL</option>
	  <option value="MLB" selected>MLB</option>
	  <option value="NBA">NBA</option>
	</select>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	sports := parseDKNetworkSports(doc)
	if len(sports) < 3 {
		t.Fatalf("sports=%d want at least NFL/MLB/NBA, got %+v", len(sports), sports)
	}
	byLabel := map[string]dkSport{}
	for _, s := range sports {
		byLabel[s.Label] = s
	}
	if byLabel["MLB"].Value != "MLB" || byLabel["MLB"].Alt != "84240" {
		t.Fatalf("MLB sport = %+v", byLabel["MLB"])
	}
	if byLabel["NFL"].Value != "NFL" || byLabel["NFL"].Alt != "88808" {
		t.Fatalf("NFL sport = %+v", byLabel["NFL"])
	}
	if _, ok := byLabel["Sports"]; ok {
		t.Fatal("placeholder Sports should be skipped")
	}

	numeric := `
	<select name="tb_eg">
	  <option value="0">Sports</option>
	  <option value="88808">NFL</option>
	  <option value="84240">MLB</option>
	</select>`
	doc2, err := goquery.NewDocumentFromReader(strings.NewReader(numeric))
	if err != nil {
		t.Fatal(err)
	}
	got := parseDKNetworkSports(doc2)
	if len(got) != 2 {
		t.Fatalf("numeric sports=%d %+v", len(got), got)
	}
}

func TestDKNetworkFallbackSportsNotMLBOnly(t *testing.T) {
	sports := dkNetworkFallbackSports()
	if len(sports) < 8 {
		t.Fatalf("fallback too small: %d", len(sports))
	}
	seen := map[string]bool{}
	for _, s := range sports {
		seen[s.Label] = true
	}
	for _, want := range []string{"NFL", "MLB", "NBA", "NHL", "NCAA Football"} {
		if !seen[want] {
			t.Fatalf("missing fallback sport %s", want)
		}
	}
}

func TestParseDKNetworkGamePanthersBuccaneers(t *testing.T) {
	html := dkNetworkGameHTML(
		"CAR Panthers @ TB Buccaneers",
		"32225607",
		"1/3, 04:30PM",
		[]dkTestMarket{
			{
				name: "Moneyline",
				rows: []dkTestRow{
					{label: "TB Buccaneers", odds: "−142", handle: "38%", bets: "44%"},
					{label: "CAR Panthers", odds: "+120", handle: "62%", bets: "56%"},
				},
			},
			{
				name: "Spread",
				rows: []dkTestRow{
					{label: "TB Buccaneers -2.5", odds: "−118", handle: "37%", bets: "46%"},
					{label: "CAR Panthers +2.5", odds: "−102", handle: "63%", bets: "54%"},
				},
			},
			{
				name: "Total",
				rows: []dkTestRow{
					{label: "Over 43.5", odds: "−115", handle: "36%", bets: "71%"},
					{label: "Under 43.5", odds: "−105", handle: "64%", bets: "29%"},
				},
			},
		},
	)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	g, ok := parseDKNetworkGame(doc.Find(".tb-se"), now, "http://example", "dk-network", "DraftKings Network", "NFL")
	if !ok {
		t.Fatal("expected parse ok")
	}
	if g.AwayTeam != "CAR Panthers" || g.HomeTeam != "TB Buccaneers" {
		t.Fatalf("teams %q @ %q", g.AwayTeam, g.HomeTeam)
	}
	if g.League != "NFL" || g.Book != "DraftKings" || g.ExternalID != "32225607" {
		t.Fatalf("meta %+v", g)
	}
	by := map[models.Market]models.MarketSplit{}
	for _, m := range g.Markets {
		by[m.Market] = m
	}
	ml := by[models.MarketMoneyline]
	if len(ml.Sides) != 2 {
		t.Fatalf("ml sides %+v", ml.Sides)
	}
	awayML := sideBy(ml.Sides, models.SideAway)
	homeML := sideBy(ml.Sides, models.SideHome)
	if awayML.BetPct == nil || *awayML.BetPct != 56 || awayML.MoneyPct == nil || *awayML.MoneyPct != 62 {
		t.Fatalf("away ML %+v", awayML)
	}
	if homeML.Odds == nil || *homeML.Odds != -142 {
		t.Fatalf("home ML odds %+v", homeML)
	}
	spread := by[models.MarketSpread]
	awaySp := sideBy(spread.Sides, models.SideAway)
	if awaySp.Line == nil || *awaySp.Line != 2.5 || awaySp.BetPct == nil || *awaySp.BetPct != 54 {
		t.Fatalf("away spread %+v", awaySp)
	}
	total := by[models.MarketTotal]
	over := sideBy(total.Sides, models.SideOver)
	if over.Line == nil || *over.Line != 43.5 || over.MoneyPct == nil || *over.MoneyPct != 36 {
		t.Fatalf("over %+v", over)
	}
}

func TestParseDKNetworkRunLineAndSoccerPrimaryPair(t *testing.T) {
	html := dkNetworkGameHTML(
		"Rangers vs Jagiellonia Bialystok",
		"99",
		"",
		[]dkTestMarket{
			{
				name: "Moneyline",
				rows: []dkTestRow{
					{label: "Rangers", odds: "-200", handle: "91%", bets: "89%"},
					{label: "Jagiellonia Bialystok", odds: "+450", handle: "9%", bets: "11%"},
				},
			},
			{
				name: "Spread",
				rows: []dkTestRow{
					{label: "Rangers -1", odds: "-110", handle: "91%", bets: "33%"},
					{label: "Rangers -1.25", odds: "-105", handle: "9%", bets: "67%"},
					{label: "Rangers -1.5", odds: "+100", handle: "0%", bets: "0%"},
					{label: "Jagiellonia Bialystok +1", odds: "-110", handle: "0%", bets: "0%"},
					{label: "Jagiellonia Bialystok +1.25", odds: "-105", handle: "0%", bets: "0%"},
					{label: "Jagiellonia Bialystok +0.75", odds: "+120", handle: "0%", bets: "0%"},
				},
			},
			{
				name: "Total",
				rows: []dkTestRow{
					{label: "Over 2.5", odds: "-110", handle: "0%", bets: "0%"},
					{label: "Over 3", odds: "-105", handle: "100%", bets: "100%"},
					{label: "Over 3.25", odds: "+100", handle: "0%", bets: "0%"},
					{label: "Under 2.5", odds: "-110", handle: "0%", bets: "0%"},
					{label: "Under 3", odds: "-105", handle: "0%", bets: "0%"},
					{label: "Under 3.25", odds: "+100", handle: "0%", bets: "0%"},
				},
			},
		},
	)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	g, ok := parseDKNetworkGame(doc.Find(".tb-se"), time.Now().UTC(), "http://example", "dk-network", "DraftKings Network", "Europa League")
	if !ok {
		t.Fatal("expected parse ok")
	}
	if g.AwayTeam != "Rangers" || g.HomeTeam != "Jagiellonia Bialystok" {
		t.Fatalf("teams %q vs %q", g.AwayTeam, g.HomeTeam)
	}
	if g.League != "Europa League" {
		t.Fatalf("league %q", g.League)
	}
	by := map[models.Market]models.MarketSplit{}
	for _, m := range g.Markets {
		by[m.Market] = m
	}
	spread := sideBy(by[models.MarketSpread].Sides, models.SideAway)
	if spread.Line == nil || *spread.Line != -1 {
		t.Fatalf("expected primary spread -1, got %+v", spread)
	}
	over := sideBy(by[models.MarketTotal].Sides, models.SideOver)
	if over.Line == nil || *over.Line != 3 {
		t.Fatalf("expected primary total 3, got %+v", over)
	}
}

func TestParseDKNetworkDocumentMultipleGames(t *testing.T) {
	html := `<div>` + dkNetworkGameHTML("CLE Guardians @ DET Tigers", "1", "", []dkTestMarket{
		{name: "Run Line", rows: []dkTestRow{
			{label: "CLE Guardians -1.5", odds: "-150", handle: "84%", bets: "34%"},
			{label: "DET Tigers +1.5", odds: "+130", handle: "16%", bets: "66%"},
		}},
	}) + dkNetworkGameHTML("SEA Mariners @ NY Yankees", "2", "", []dkTestMarket{
		{name: "Moneyline", rows: []dkTestRow{
			{label: "NY Yankees", odds: "-140", handle: "54%", bets: "80%"},
			{label: "SEA Mariners", odds: "+120", handle: "46%", bets: "20%"},
		}},
	}) + `</div>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	games := parseDKNetworkDocument(doc, time.Now().UTC(), "http://example", "dk-network", "DraftKings Network", "MLB")
	if len(games) != 2 {
		t.Fatalf("games=%d", len(games))
	}
	if games[0].Markets[0].Market != models.MarketSpread {
		t.Fatalf("run line should map to spread, got %s", games[0].Markets[0].Market)
	}
}

type dkTestRow struct {
	label, odds, handle, bets string
}

type dkTestMarket struct {
	name string
	rows []dkTestRow
}

func dkNetworkGameHTML(title, eventID, when string, markets []dkTestMarket) string {
	var b strings.Builder
	b.WriteString(`<div class="tb-se"><div class="tb-se-title"><h5 class="tb-se-title-new">`)
	b.WriteString(`<a href="https://sportsbook.draftkings.com/event/`)
	b.WriteString(eventID)
	b.WriteString(`">`)
	b.WriteString(title)
	b.WriteString(`</a></h5>`)
	if when != "" {
		b.WriteString(`<span>`)
		b.WriteString(when)
		b.WriteString(`</span>`)
	}
	b.WriteString(`</div><div class="tb-market-wrap">`)
	for _, m := range markets {
		b.WriteString(`<div><div class="tb-se-head"><div>`)
		b.WriteString(m.name)
		b.WriteString(`</div><div>Odds</div><div>% Handle</div><div>% Bets</div></div><div class="tb-sm">`)
		for _, r := range m.rows {
			b.WriteString(`<div class="tb-sodd"><div class="tb-slipline">`)
			b.WriteString(r.label)
			b.WriteString(`</div><div><a class="tb-odd-s">`)
			b.WriteString(r.odds)
			b.WriteString(`</a></div><div>`)
			b.WriteString(r.handle)
			b.WriteString(`<div class="tb-progress"><div></div></div></div><div>`)
			b.WriteString(r.bets)
			b.WriteString(`<div class="tb-progress"><div></div></div></div></div>`)
		}
		b.WriteString(`</div></div>`)
	}
	b.WriteString(`</div></div>`)
	return b.String()
}

func sideBy(sides []models.SideSplit, side models.Side) models.SideSplit {
	for _, s := range sides {
		if s.Side == side {
			return s
		}
	}
	return models.SideSplit{}
}
