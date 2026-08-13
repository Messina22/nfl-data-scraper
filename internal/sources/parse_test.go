package sources

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
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
