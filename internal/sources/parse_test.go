package sources

import (
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

func intPtr(v int) *int { return &v }
