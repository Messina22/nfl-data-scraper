package sources

import (
	"strings"
	"testing"
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
