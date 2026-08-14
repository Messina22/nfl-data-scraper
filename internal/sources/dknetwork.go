package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"nfl-data-scraper/internal/httpx"
	"nfl-data-scraper/internal/models"
)

const (
	dkNetworkSplitsPath = "https://dknetwork.draftkings.com/draftkings-sportsbook-betting-splits/"
	dkNetworkDateRange  = "n30days"
	dkNetworkAllMarkets = "0"
	dkNetworkMaxPages   = 10
	dkNetworkWorkers    = 4
)

// DKNetwork collects first-party DraftKings Sportsbook betting splits
// (bets % + handle %) from the DK Network board across every listed sport.
type DKNetwork struct {
	client *httpx.Client
}

func NewDKNetwork() *DKNetwork {
	c := httpx.New(45 * time.Second)
	c.Referer = dkNetworkSplitsPath
	c.Origin = "https://dknetwork.draftkings.com"
	return &DKNetwork{client: c}
}

func (d *DKNetwork) ID() string   { return "dk-network" }
func (d *DKNetwork) Name() string { return "DraftKings Network" }

// dkSport is one board filter: tb_eg value plus a numeric/slug alternate.
type dkSport struct {
	Label string
	Value string
	Alt   string
}

type dkSportResult struct {
	games []models.GameSplits
	err   error
	label string
}

func (d *DKNetwork) Collect(ctx context.Context) ([]models.GameSplits, error) {
	sports, err := d.discoverSports(ctx)
	if err != nil {
		sports = dkNetworkFallbackSports()
	}
	if len(sports) == 0 {
		sports = dkNetworkFallbackSports()
	}

	sem := make(chan struct{}, dkNetworkWorkers)
	var wg sync.WaitGroup
	ch := make(chan dkSportResult, len(sports))
	for _, sp := range sports {
		wg.Add(1)
		go func(sp dkSport) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				ch <- dkSportResult{err: ctx.Err(), label: sp.Label}
				return
			}
			defer func() { <-sem }()
			games, err := d.collectSport(ctx, sp)
			ch <- dkSportResult{games: games, err: err, label: sp.Label}
		}(sp)
	}
	wg.Wait()
	close(ch)

	var results []dkSportResult
	for r := range ch {
		results = append(results, r)
	}
	return combineDKNetworkResults(results)
}

func combineDKNetworkResults(results []dkSportResult) ([]models.GameSplits, error) {
	var out []models.GameSplits
	var errs []string
	for _, r := range results {
		out = append(out, r.games...)
		if r.err != nil {
			label := r.label
			if label == "" {
				label = "sport"
			}
			errs = append(errs, label+": "+r.err.Error())
		}
	}
	if len(out) == 0 {
		if len(errs) > 0 {
			return nil, fmt.Errorf("no DraftKings Network splits found (%s)", strings.Join(errs, "; "))
		}
		return nil, fmt.Errorf("no DraftKings Network splits rows found (board may be empty or markup changed)")
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("partial DraftKings Network collect (%s)", strings.Join(errs, "; "))
	}
	return out, nil
}

func (d *DKNetwork) discoverSports(ctx context.Context) ([]dkSport, error) {
	body, _, err := d.client.Get(ctx, dkNetworkSplitsPath)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse dk network html: %w", err)
	}
	sports := parseDKNetworkSports(doc)
	if len(sports) == 0 {
		return nil, fmt.Errorf("no sports in #sid-list")
	}
	return sports, nil
}

func (d *DKNetwork) collectSport(ctx context.Context, sp dkSport) ([]models.GameSplits, error) {
	games, err := d.collectSportValue(ctx, sp, sp.Value)
	if err == nil && len(games) > 0 {
		return games, nil
	}
	if sp.Alt != "" && sp.Alt != sp.Value {
		altGames, altErr := d.collectSportValue(ctx, sp, sp.Alt)
		if altErr == nil && len(altGames) > 0 {
			return altGames, nil
		}
		if len(altGames) > len(games) {
			return altGames, altErr
		}
		if err == nil {
			err = altErr
		}
	}
	return games, err
}

func (d *DKNetwork) collectSportValue(ctx context.Context, sp dkSport, eg string) ([]models.GameSplits, error) {
	var out []models.GameSplits
	seen := map[string]bool{}
	for page := 1; page <= dkNetworkMaxPages; page++ {
		pageURL := dkNetworkPageURL(eg, sp.Label, page)
		body, finalURL, err := d.client.Get(ctx, pageURL)
		if err != nil {
			return out, err
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if err != nil {
			return out, fmt.Errorf("parse html: %w", err)
		}
		if dkNetworkFetchForbidden(doc) && doc.Find(".tb-se").Length() == 0 {
			return out, fmt.Errorf("unable to fetch data from server (403)")
		}
		now := time.Now().UTC()
		pageGames := parseDKNetworkDocument(doc, now, finalURL, d.ID(), d.Name(), sp.Label)
		if len(pageGames) == 0 {
			break
		}
		added := 0
		for _, g := range pageGames {
			key := g.ExternalID
			if key == "" {
				key = g.AwayTeam + "|" + g.HomeTeam + "|" + fmt.Sprint(g.StartTime)
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, g)
			added++
		}
		if added == 0 {
			break
		}
		if !dkNetworkHasNextPage(doc, page) {
			break
		}
	}
	return out, nil
}

func dkNetworkPageURL(eg, label string, page int) string {
	q := url.Values{}
	q.Set("tb_eg", eg)
	q.Set("tb_edate", dkNetworkDateRange)
	q.Set("tb_emt", dkNetworkAllMarkets)
	if strings.TrimSpace(label) != "" {
		q.Set("itm_content", label)
	}
	if page > 1 {
		q.Set("tb_page", strconv.Itoa(page))
	}
	return dkNetworkSplitsPath + "?" + q.Encode()
}

func parseDKNetworkSports(doc *goquery.Document) []dkSport {
	var out []dkSport
	seen := map[string]bool{}
	doc.Find("#sid-list option, select[name='tb_eg'] option").Each(func(_ int, opt *goquery.Selection) {
		val := strings.TrimSpace(opt.AttrOr("value", ""))
		label := strings.TrimSpace(opt.Text())
		if val == "" || val == "0" || strings.EqualFold(val, "Sports") || strings.EqualFold(label, "Sports") {
			return
		}
		if label == "" {
			label = val
		}
		if seen[val] {
			return
		}
		seen[val] = true
		out = append(out, dkSport{
			Label: label,
			Value: val,
			Alt:   dkNetworkAltEG(label, val),
		})
	})
	return out
}

func dkNetworkAltEG(label, value string) string {
	id := dkEventGroupID(label)
	if id != "" && id != value {
		return id
	}
	if slug := dkEventGroupSlug(value); slug != "" && slug != value {
		return slug
	}
	if !isAllDigits(value) {
		if id := dkEventGroupID(value); id != "" && id != value {
			return id
		}
	}
	return ""
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func dkNetworkFallbackSports() []dkSport {
	labels := []string{
		"NFL",
		"NFL Preseason",
		"NBA",
		"NHL",
		"MLB",
		"WNBA",
		"UFC",
		"UFL",
		"NCAA Football",
		"NCAA Basketball",
		"NCAA Womens Basketball",
		"NCAA Baseball",
		"NCAA Ice Hockey",
		"England Premier League",
		"Champions League",
		"Europa League",
		"MLS",
	}
	out := make([]dkSport, 0, len(labels))
	for _, label := range labels {
		id := dkEventGroupID(label)
		value := label
		if id != "" {
			value = id
		}
		out = append(out, dkSport{Label: label, Value: value, Alt: dkNetworkAltEG(label, value)})
	}
	return out
}

func dkEventGroupID(label string) string {
	switch dkNormKey(label) {
	case "nfl":
		return "88808"
	case "nba":
		return "42648"
	case "nhl":
		return "42133"
	case "mlb":
		return "84240"
	case "wnba":
		return "94682"
	case "ufc":
		return "9034"
	case "ncaafootball", "ncaaf", "cfb":
		return "87637"
	case "ncaabasketball", "ncaab", "cbb":
		return "92483"
	case "ncaawomensbasketball", "ncaaw":
		return "36647"
	case "ncaabaseball":
		return "41151"
	case "ncaaicehockey":
		return "84813"
	case "englandpremierleague", "epl", "premierleague":
		return "40253"
	case "championsleague", "ucl":
		return "40685"
	case "europaleague":
		return "41410"
	case "mls":
		return "89345"
	default:
		return ""
	}
}

func dkEventGroupSlug(id string) string {
	switch id {
	case "88808":
		return "NFL"
	case "42648":
		return "NBA"
	case "42133":
		return "NHL"
	case "84240":
		return "MLB"
	case "94682":
		return "WNBA"
	case "9034":
		return "UFC"
	case "87637":
		return "NCAA Football"
	case "92483":
		return "NCAA Basketball"
	case "36647":
		return "NCAA Womens Basketball"
	case "41151":
		return "NCAA Baseball"
	case "84813":
		return "NCAA Ice Hockey"
	case "40253":
		return "England Premier League"
	case "40685":
		return "Champions League"
	case "41410":
		return "Europa League"
	case "89345":
		return "MLS"
	default:
		return ""
	}
}

func dkNormKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func dkNetworkFetchForbidden(doc *goquery.Document) bool {
	txt := strings.ToLower(doc.Find("#tbsedid, .wrap-for-export").Text())
	return strings.Contains(txt, "unable to fetch data from server") && strings.Contains(txt, "403")
}

func dkNetworkHasNextPage(doc *goquery.Document, current int) bool {
	found := false
	doc.Find(".tb_pagination a[href]").Each(func(_ int, a *goquery.Selection) {
		if a.HasClass("pg_disabled") {
			return
		}
		href := a.AttrOr("href", "")
		u, err := url.Parse(href)
		if err != nil {
			return
		}
		p, _ := strconv.Atoi(u.Query().Get("tb_page"))
		if p > current {
			found = true
		}
	})
	return found
}

func parseDKNetworkDocument(doc *goquery.Document, now time.Time, pageURL, sourceID, sourceName, league string) []models.GameSplits {
	var out []models.GameSplits
	doc.Find(".tb-se").Each(func(_ int, se *goquery.Selection) {
		if g, ok := parseDKNetworkGame(se, now, pageURL, sourceID, sourceName, league); ok {
			out = append(out, g)
		}
	})
	return out
}

func parseDKNetworkGame(se *goquery.Selection, now time.Time, pageURL, sourceID, sourceName, league string) (models.GameSplits, bool) {
	titleSel := se.Find(".tb-se-title-new").First()
	title := compactSpace(titleSel.Text())
	away, home := splitDKMatchup(title)
	if away == "" || home == "" {
		return models.GameSplits{}, false
	}
	href, _ := titleSel.Find("a").Attr("href")
	extID := dkEventIDFromURL(href)
	start := parseDKNetworkGameTime(compactSpace(se.Find(".tb-se-title > span").First().Text()), now)

	var markets []models.MarketSplit
	se.Find(".tb-market-wrap > div").Each(func(_ int, wrap *goquery.Selection) {
		head := compactSpace(wrap.Find(".tb-se-head > div").First().Text())
		kind := dkMarketKind(head)
		if kind == "" {
			return
		}
		var rows []dkSplitRow
		wrap.Find(".tb-sodd").Each(func(_ int, row *goquery.Selection) {
			if r, ok := parseDKNetworkRow(row); ok {
				rows = append(rows, r)
			}
		})
		if ms, ok := dkMarketFromRows(kind, away, home, rows); ok {
			markets = append(markets, ms)
		}
	})

	gs := models.GameSplits{
		SourceID:   sourceID,
		SourceName: sourceName,
		Book:       "DraftKings",
		League:     league,
		ExternalID: extID,
		StartTime:  start,
		AwayTeam:   away,
		HomeTeam:   home,
		FetchedAt:  now,
		URL:        pageURL,
		Markets:    markets,
	}
	if !hasAnySplit(gs) {
		return models.GameSplits{}, false
	}
	return gs, true
}

type dkSplitRow struct {
	label  string
	line   *float64
	odds   *int
	handle *float64
	bets   *float64
}

func parseDKNetworkRow(row *goquery.Selection) (dkSplitRow, bool) {
	label := compactSpace(row.Find(".tb-slipline").First().Text())
	if label == "" {
		return dkSplitRow{}, false
	}
	r := dkSplitRow{
		label: label,
		odds:  parseAmericanOdds(row.Find(".tb-odd-s").First().Text()),
	}
	cells := row.ChildrenFiltered("div")
	// 0 slipline, 1 odds, 2 handle, 3 bets
	if cells.Length() >= 4 {
		r.handle = parseFirstPct(cells.Eq(2).Text())
		r.bets = parseFirstPct(cells.Eq(3).Text())
	} else {
		// Fallback: first two percents in the row.
		re := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)
		ms := re.FindAllStringSubmatch(row.Text(), 2)
		if len(ms) >= 1 {
			r.handle = parsePct(ms[0][0])
		}
		if len(ms) >= 2 {
			r.bets = parsePct(ms[1][0])
		}
	}
	r.line = dkLineFromLabel(label)
	if r.handle == nil && r.bets == nil {
		return dkSplitRow{}, false
	}
	return r, true
}

var firstPctRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)

func parseFirstPct(s string) *float64 {
	m := firstPctRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	return parsePct(m[0])
}

func dkMarketKind(name string) models.Market {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(n, "money"):
		return models.MarketMoneyline
	case strings.Contains(n, "total"):
		return models.MarketTotal
	case strings.Contains(n, "spread"), strings.Contains(n, "run line"), strings.Contains(n, "puck"):
		return models.MarketSpread
	default:
		return ""
	}
}

func dkMarketFromRows(kind models.Market, away, home string, rows []dkSplitRow) (models.MarketSplit, bool) {
	if len(rows) == 0 {
		return models.MarketSplit{}, false
	}
	var sides []models.SideSplit
	if kind == models.MarketTotal {
		sides = pickDKTotalSides(rows)
	} else {
		sides = pickDKTeamSides(kind, away, home, rows)
	}
	if len(sides) == 0 {
		return models.MarketSplit{}, false
	}
	return models.MarketSplit{Market: kind, Sides: sides}, true
}

func pickDKTotalSides(rows []dkSplitRow) []models.SideSplit {
	type pair struct {
		over, under *dkSplitRow
		action      float64
	}
	byLine := map[string]*pair{}
	var unlined pair
	for i := range rows {
		r := &rows[i]
		over := dkIsOver(r.label)
		under := dkIsUnder(r.label)
		if !over && !under {
			continue
		}
		key := "na"
		if r.line != nil {
			key = strconv.FormatFloat(*r.line, 'f', -1, 64)
		}
		p := byLine[key]
		if p == nil {
			p = &pair{}
			byLine[key] = p
		}
		if over {
			p.over = r
		} else {
			p.under = r
		}
		p.action += dkRowAction(r)
		_ = unlined
	}
	var best *pair
	for _, p := range byLine {
		if p.over == nil && p.under == nil {
			continue
		}
		if best == nil || p.action > best.action {
			best = p
		}
	}
	if best == nil {
		return nil
	}
	var sides []models.SideSplit
	if best.over != nil {
		sides = append(sides, sideFrom("Over", models.SideOver, best.over.line, best.over.handle, best.over.bets, best.over.odds))
	}
	if best.under != nil {
		sides = append(sides, sideFrom("Under", models.SideUnder, best.under.line, best.under.handle, best.under.bets, best.under.odds))
	}
	return sides
}

func pickDKTeamSides(kind models.Market, away, home string, rows []dkSplitRow) []models.SideSplit {
	type pair struct {
		away, home *dkSplitRow
		action     float64
	}
	byLine := map[string]*pair{}
	for i := range rows {
		r := &rows[i]
		team := dkTeamFromLabel(r.label)
		isAway := dkTeamsMatch(team, away)
		isHome := dkTeamsMatch(team, home)
		if isAway && isHome {
			// Ambiguous (shared tokens); prefer the longer remaining name match.
			if len(dkNormKey(away)) >= len(dkNormKey(home)) {
				isHome = false
			} else {
				isAway = false
			}
		}
		if !isAway && !isHome {
			continue
		}
		key := "ml"
		if kind == models.MarketSpread {
			key = "na"
			if r.line != nil {
				key = strconv.FormatFloat(absFloat(*r.line), 'f', -1, 64)
			}
		}
		p := byLine[key]
		if p == nil {
			p = &pair{}
			byLine[key] = p
		}
		if isAway {
			p.away = r
		} else {
			p.home = r
		}
		p.action += dkRowAction(r)
	}
	var best *pair
	for _, p := range byLine {
		if best == nil || p.action > best.action {
			best = p
		}
	}
	if best == nil {
		return nil
	}
	var sides []models.SideSplit
	if best.away != nil {
		label := dkTeamFromLabel(best.away.label)
		if label == "" {
			label = away
		}
		sides = append(sides, sideFrom(label, models.SideAway, best.away.line, best.away.handle, best.away.bets, best.away.odds))
	}
	if best.home != nil {
		label := dkTeamFromLabel(best.home.label)
		if label == "" {
			label = home
		}
		sides = append(sides, sideFrom(label, models.SideHome, best.home.line, best.home.handle, best.home.bets, best.home.odds))
	}
	return sides
}

func dkRowAction(r *dkSplitRow) float64 {
	var n float64
	if r.handle != nil {
		n += *r.handle
	}
	if r.bets != nil {
		n += *r.bets
	}
	return n
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func dkIsOver(label string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(label)), "over")
}

func dkIsUnder(label string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(label)), "under")
}

var trailingLineRe = regexp.MustCompile(`\s*([+-]?\d+(?:\.\d+)?)\s*$`)

func dkLineFromLabel(label string) *float64 {
	s := strings.TrimSpace(label)
	low := strings.ToLower(s)
	if dkIsOver(low) || dkIsUnder(low) {
		fields := strings.Fields(s)
		if len(fields) >= 2 {
			return parseLine(fields[len(fields)-1])
		}
		return nil
	}
	m := trailingLineRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	// Don't treat a lone team number-like token as a line (rare). Require a sign or a decimal
	// or that the remainder still looks like a team name.
	rest := strings.TrimSpace(s[:len(s)-len(m[0])])
	if rest == "" {
		return nil
	}
	return parseLine(m[1])
}

func dkTeamFromLabel(label string) string {
	s := strings.TrimSpace(label)
	if dkIsOver(s) || dkIsUnder(s) {
		return s
	}
	m := trailingLineRe.FindStringSubmatch(s)
	if m != nil {
		rest := strings.TrimSpace(s[:len(s)-len(m[0])])
		if rest != "" {
			return rest
		}
	}
	return s
}

func dkTeamsMatch(a, b string) bool {
	x := dkNormKey(a)
	y := dkNormKey(b)
	if x == "" || y == "" {
		return false
	}
	return x == y || strings.Contains(x, y) || strings.Contains(y, x)
}

func splitDKMatchup(title string) (away, home string) {
	title = compactSpace(title)
	for _, sep := range []string{" @ ", " vs. ", " vs "} {
		if i := strings.Index(strings.ToLower(title), strings.ToLower(sep)); i >= 0 {
			away = strings.TrimSpace(title[:i])
			home = strings.TrimSpace(title[i+len(sep):])
			return away, home
		}
	}
	return "", ""
}

func dkEventIDFromURL(href string) string {
	re := regexp.MustCompile(`/event/(\d+)`)
	m := re.FindStringSubmatch(href)
	if m == nil {
		return ""
	}
	return m[1]
}

func parseDKNetworkGameTime(s string, now time.Time) *time.Time {
	s = compactSpace(s)
	if s == "" {
		return nil
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.FixedZone("ET", -5*3600)
	}
	nowET := now.In(loc)
	layouts := []string{"1/2, 03:04PM", "1/2, 3:04PM", "1/2, 03:04 PM", "1/2, 3:04 PM"}
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, s, loc)
		if err != nil {
			continue
		}
		t = t.AddDate(nowET.Year(), 0, 0)
		// If the parsed date is far in the past, it belongs to next year (Dec → Jan).
		if nowET.Sub(t) > 14*24*time.Hour {
			t = t.AddDate(1, 0, 0)
		}
		if t.Sub(nowET) > 320*24*time.Hour {
			t = t.AddDate(-1, 0, 0)
		}
		utc := t.UTC()
		return &utc
	}
	return nil
}

func compactSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
