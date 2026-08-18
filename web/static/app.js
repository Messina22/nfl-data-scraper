(() => {
  const board = document.getElementById("board");
  const statusMeta = document.getElementById("statusMeta");
  const sourcePills = document.getElementById("sourcePills");
  const sourceFilter = document.getElementById("sourceFilter");
  const leagueFilter = document.getElementById("leagueFilter");
  const windowFilter = document.getElementById("windowFilter");
  const refreshBtn = document.getElementById("refreshBtn");

  let snapshot = { games: [], sources: [], collected_at: null };

  function pct(v) {
    if (v == null || Number.isNaN(Number(v))) return null;
    return Math.round(Number(v));
  }

  function fmtPct(v) {
    const p = pct(v);
    return p == null ? "—" : `${p}%`;
  }

  function fmtLine(side) {
    if (side.odds != null) {
      const n = Number(side.odds);
      return n > 0 ? `+${n}` : `${n}`;
    }
    if (side.line == null) return "";
    const n = Number(side.line);
    if (side.side === "over") return `O ${n}`;
    if (side.side === "under") return `U ${n}`;
    return n > 0 ? `+${n}` : `${n}`;
  }

  const SOURCE_ICON = {
    "action-network": "/img/sources/action-network.jpg",
    "vsin-dk": "/img/sources/vsin.png",
    "vsin-circa": "/img/sources/vsin.png",
    "covers-consensus": "/img/sources/covers.ico",
  };
  function sourceIconHtml(sourceId) {
    const src = SOURCE_ICON[sourceId];
    return src ? `<img class="source-icon" src="${src}" alt="" />` : "";
  }

  // Canonical NFL abbrs plus common publisher short forms → abbr.
  const NFL_TEAM_ABBR = (() => {
    const teams = {
      ARI: ["Arizona Cardinals", "Cardinals", "ARI"],
      ATL: ["Atlanta Falcons", "Falcons", "ATL"],
      BAL: ["Baltimore Ravens", "Ravens", "BAL"],
      BUF: ["Buffalo Bills", "Bills", "BUF"],
      CAR: ["Carolina Panthers", "Panthers", "CAR"],
      CHI: ["Chicago Bears", "Bears", "CHI"],
      CIN: ["Cincinnati Bengals", "Bengals", "CIN"],
      CLE: ["Cleveland Browns", "Browns", "CLE"],
      DAL: ["Dallas Cowboys", "Cowboys", "DAL"],
      DEN: ["Denver Broncos", "Broncos", "DEN"],
      DET: ["Detroit Lions", "Lions", "DET"],
      GB: ["Green Bay Packers", "Packers", "GB", "GNB"],
      HOU: ["Houston Texans", "Texans", "HOU"],
      IND: ["Indianapolis Colts", "Colts", "IND"],
      JAX: ["Jacksonville Jaguars", "Jaguars", "JAX", "JAC"],
      KC: ["Kansas City Chiefs", "Chiefs", "KC", "KAN"],
      LV: ["Las Vegas Raiders", "Raiders", "LV", "LVR", "Oakland Raiders"],
      LAC: ["Los Angeles Chargers", "LA Chargers", "Chargers", "LAC"],
      LAR: ["Los Angeles Rams", "LA Rams", "Rams", "LAR"],
      MIA: ["Miami Dolphins", "Dolphins", "MIA"],
      MIN: ["Minnesota Vikings", "Vikings", "MIN"],
      NE: ["New England Patriots", "Patriots", "NE", "NWE"],
      NO: ["New Orleans Saints", "Saints", "NO", "NOR"],
      NYG: ["New York Giants", "NY Giants", "Giants", "NYG"],
      NYJ: ["New York Jets", "NY Jets", "Jets", "NYJ"],
      PHI: ["Philadelphia Eagles", "Eagles", "PHI"],
      PIT: ["Pittsburgh Steelers", "Steelers", "PIT"],
      SF: ["San Francisco 49ers", "49ers", "SF", "SFO"],
      SEA: ["Seattle Seahawks", "Seahawks", "SEA"],
      TB: ["Tampa Bay Buccaneers", "Buccaneers", "Bucs", "TB", "TAM"],
      TEN: ["Tennessee Titans", "Titans", "TEN"],
      WAS: [
        "Washington Commanders",
        "Wash Commanders",
        "Commanders",
        "Washington",
        "WAS",
        "WSH",
      ],
    };
    const map = Object.create(null);
    for (const [abbr, aliases] of Object.entries(teams)) {
      for (const alias of aliases) {
        map[normalizeTeamKey(alias)] = abbr;
      }
      map[normalizeTeamKey(abbr)] = abbr;
    }
    return map;
  })();
  const NFL_CANONICAL = new Set(Object.values(NFL_TEAM_ABBR));

  function normalizeTeamKey(name) {
    return (name || "").toLowerCase().replace(/[^a-z0-9]/g, "");
  }

  function isNflLeague(league) {
    const x = (league || "").toLowerCase();
    return !x || x === "nfl" || x === "nfl preseason";
  }

  function groupingLeague(league) {
    if (isNflLeague(league)) return "nfl";
    return (league || "").toLowerCase();
  }

  function teamAbbr(abbr, name, league) {
    // Prefer a known abbr, but fall through to the display name when the
    // publisher uses an ambiguous code (e.g. Action Network "LA" for Rams).
    // Only apply the NFL alias map to NFL (or untagged) reports so MLB/NBA
    // teams like CLE / DET / SEA do not collapse onto Browns / Lions / Seahawks.
    if (isNflLeague(league)) {
      if (abbr) {
        const fromAbbr = NFL_TEAM_ABBR[normalizeTeamKey(abbr)];
        if (fromAbbr) return fromAbbr;
      }
      const fromName = NFL_TEAM_ABBR[normalizeTeamKey(name)];
      if (fromName) return fromName;
    }
    if (abbr) return String(abbr).toUpperCase();
    // Never drop a report: fall back to normalized display name.
    return normalizeTeamKey(name) || "unk";
  }

  function espnNflSlug(abbr) {
    if (abbr === "WAS") return "wsh";
    return String(abbr).toLowerCase();
  }

  function teamLogoUrl(abbr, name, league) {
    if (!isNflLeague(league)) return "";
    const canon = teamAbbr(abbr, name, league);
    if (!NFL_CANONICAL.has(canon)) return "";
    return `https://a.espncdn.com/i/teamlogos/nfl/500/${espnNflSlug(canon)}.png`;
  }

  function teamChip(abbr, name, league) {
    const label = escapeHtml(name || abbr || "");
    const src = teamLogoUrl(abbr, name, league);
    const img = src
      ? `<img class="team-logo" src="${escapeHtml(src)}" alt="" onerror="this.hidden=true" />`
      : "";
    return `<span class="team-chip">${img}<span>${label}</span></span>`;
  }

  function contestKey(g) {
    const league = groupingLeague(g.league);
    const day = g.start_time ? g.start_time.slice(0, 10) : "na";
    const away = teamAbbr(g.away_abbr, g.away_team, g.league);
    const home = teamAbbr(g.home_abbr, g.home_team, g.league);
    return `${league}|${day}|${away}|${home}`;
  }

  function preferDisplayName(current, next) {
    if (!current) return next || "";
    if (!next) return current;
    return next.length > current.length ? next : current;
  }

  function groupGames(games) {
    const byKey = new Map();
    for (const g of games) {
      const key = contestKey(g);
      let found = byKey.get(key);
      if (!found) {
        found = {
          key,
          away: g.away_team,
          home: g.home_team,
          awayAbbr: teamAbbr(g.away_abbr, g.away_team, g.league),
          homeAbbr: teamAbbr(g.home_abbr, g.home_team, g.league),
          league: g.league || "",
          start: g.start_time,
          reports: [],
        };
        byKey.set(key, found);
      }
      found.reports.push(g);
      found.away = preferDisplayName(found.away, g.away_team);
      found.home = preferDisplayName(found.home, g.home_team);
      if (!found.start && g.start_time) found.start = g.start_time;
      if (!found.league && g.league) found.league = g.league;
    }
    const groups = [...byKey.values()];
    for (const grp of groups) {
      grp.reports.sort((a, b) => String(a.source_id || "").localeCompare(String(b.source_id || "")));
    }
    groups.sort((a, b) => String(a.start || "").localeCompare(String(b.start || "")));
    return groups;
  }

  function inWindow(start, days) {
    if (days === "all" || !start) return true;
    const t = Date.parse(start);
    if (Number.isNaN(t)) return true;
    const now = Date.now();
    const max = now + Number(days) * 86400000;
    // Include recently started same-day games and future ones in window.
    return t >= now - 12 * 3600000 && t <= max;
  }

  function fmtEdge(v) {
    if (v == null || Number.isNaN(Number(v))) return null;
    const n = Number(v);
    const rounded = Math.round(n * 10) / 10;
    return rounded > 0 ? `+${rounded}%` : `${rounded}%`;
  }

  function insightForMarket(insights, marketName) {
    if (!insights || !insights.length) return null;
    return insights.find((i) => i.market === marketName) || null;
  }

  function insightBlock(insight) {
    if (!insight) return "";
    const edge = fmtEdge(insight.edge_pct);
    const bits = [];
    if (insight.grade) bits.push(`Grade ${escapeHtml(insight.grade)}`);
    if (edge) bits.push(`Edge ${escapeHtml(edge)}`);
    const lean = insight.label ? escapeHtml(insight.label) : escapeHtml(insight.side || "");
    return `<div class="pro-insight" title="Action PRO projection lean">
      <span class="pro-insight-label">PRO</span>
      <span class="pro-insight-lean">${lean}</span>
      ${bits.length ? `<span class="pro-insight-meta">${bits.join(" · ")}</span>` : ""}
    </div>`;
  }

  function sideLabelHtml(side, g) {
    if (side.side === "away") {
      return teamChip(g.away_abbr, side.label || g.away_team, g.league);
    }
    if (side.side === "home") {
      return teamChip(g.home_abbr, side.label || g.home_team, g.league);
    }
    return escapeHtml(side.label || side.side);
  }

  function marketBlock(market, insight, g) {
    if (!market) {
      return `<article class="market"><h4>Unavailable</h4><p class="error-note" style="padding:0.4rem 0">No data</p></article>`;
    }
    const sides = (market.sides || [])
      .map((side) => {
        const bet = pct(side.bet_pct);
        const money = pct(side.money_pct);
        return `
          <div class="side">
            <div class="side-top">
              ${sideLabelHtml(side, g)}
              <span class="side-line">${escapeHtml(fmtLine(side))}</span>
            </div>
            <div class="bars">
              <div class="bar-row">
                <span>Bets</span>
                <div class="track"><div class="fill bets" data-width="${bet ?? 0}"></div></div>
                <span>${fmtPct(side.bet_pct)}</span>
              </div>
              <div class="bar-row">
                <span>Money</span>
                <div class="track"><div class="fill money" data-width="${money ?? 0}"></div></div>
                <span>${fmtPct(side.money_pct)}</span>
              </div>
            </div>
          </div>`;
      })
      .join("");
    return `<article class="market"><h4>${escapeHtml(market.market)}</h4>${insightBlock(insight)}${sides}</article>`;
  }

  function escapeHtml(s) {
    return String(s)
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;");
  }

  function formatWhen(iso) {
    if (!iso) return "Time TBD";
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString(undefined, {
      weekday: "short",
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
  }

  function render() {
    const source = sourceFilter.value;
    const league = leagueFilter.value;
    const days = windowFilter.value;
    let games = snapshot.games || [];
    if (source !== "all") games = games.filter((g) => g.source_id === source);
    if (league !== "all") {
      games = games.filter((g) => groupingLeague(g.league) === groupingLeague(league));
    }
    games = games.filter((g) => inWindow(g.start_time, days));

    const groups = groupGames(games);
    sourcePills.innerHTML = (snapshot.sources || [])
      .map((s) => {
        const cls = s.ok ? "pill ok" : "pill";
        const detail = s.ok ? `${s.games} games` : "error";
        return `<span class="${cls}" title="${escapeHtml(s.error || "")}"><span class="dot"></span>${sourceIconHtml(s.id)}${escapeHtml(s.name)} · ${detail}</span>`;
      })
      .join("");

    const collected = snapshot.collected_at
      ? new Date(snapshot.collected_at).toLocaleString()
      : "never";
    statusMeta.textContent = `${groups.length} matchups · ${games.length} source reports · collected ${collected}`;

    if (!groups.length) {
      board.innerHTML = `<div class="empty">No splits in this window. Try “All scheduled” or refresh after sources update.</div>`;
      return;
    }

    board.innerHTML = groups
      .map((grp, idx) => {
        const reports = grp.reports
          .map((g) => {
            const byMarket = Object.fromEntries((g.markets || []).map((m) => [m.market, m]));
            const insights = g.pro_insights || [];
            return `
              <div class="source-block">
                <div class="source-label">
                  <span>${sourceIconHtml(g.source_id)}${escapeHtml(g.source_name)}${g.book ? ` · ${escapeHtml(g.book)}` : ""}</span>
                  <span>${g.num_bets != null ? `${g.num_bets.toLocaleString()} bets tracked` : ""}</span>
                </div>
                <div class="markets">
                  ${marketBlock(byMarket.spread, insightForMarket(insights, "spread"), g)}
                  ${marketBlock(byMarket.moneyline, insightForMarket(insights, "moneyline"), g)}
                  ${marketBlock(byMarket.total, insightForMarket(insights, "total"), g)}
                </div>
              </div>`;
          })
          .join("");
        return `
          <section class="matchup" style="animation-delay:${Math.min(idx * 0.04, 0.4)}s">
            <div class="matchup-head">
              <h2 class="matchup-title">${teamChip(grp.awayAbbr, grp.away, grp.league)} <span class="matchup-at">@</span> ${teamChip(grp.homeAbbr, grp.home, grp.league)}</h2>
              <div class="matchup-meta">${grp.league ? `<span class="league-tag">${escapeHtml(grp.league)}</span>` : ""}${escapeHtml(formatWhen(grp.start))}</div>
            </div>
            ${reports}
          </section>`;
      })
      .join("");

    requestAnimationFrame(() => {
      document.querySelectorAll(".fill[data-width]").forEach((el) => {
        el.style.width = `${el.getAttribute("data-width")}%`;
      });
    });
  }

  function fillSourceOptions() {
    const current = sourceFilter.value;
    const opts = [`<option value="all">All sources</option>`];
    for (const s of snapshot.sources || []) {
      opts.push(`<option value="${escapeHtml(s.id)}">${escapeHtml(s.name)}</option>`);
    }
    sourceFilter.innerHTML = opts.join("");
    if ([...sourceFilter.options].some((o) => o.value === current)) {
      sourceFilter.value = current;
    }
  }

  function fillLeagueOptions() {
    const current = leagueFilter.value;
    const seen = new Map();
    for (const g of snapshot.games || []) {
      let name = (g.league || "").trim();
      if (!name) continue;
      if (isNflLeague(name)) name = "NFL";
      const key = groupingLeague(name);
      if (!seen.has(key)) seen.set(key, name);
    }
    const opts = [`<option value="all">All sports</option>`];
    [...seen.values()]
      .sort((a, b) => a.localeCompare(b))
      .forEach((name) => {
        opts.push(`<option value="${escapeHtml(name)}">${escapeHtml(name)}</option>`);
      });
    leagueFilter.innerHTML = opts.join("");
    if ([...leagueFilter.options].some((o) => o.value === current)) {
      leagueFilter.value = current;
    }
  }

  async function load() {
    const res = await fetch("/api/splits");
    snapshot = await res.json();
    fillSourceOptions();
    fillLeagueOptions();
    render();
  }

  async function refresh() {
    refreshBtn.disabled = true;
    refreshBtn.textContent = "Refreshing…";
    const previous = snapshot.collected_at;
    try {
      const res = await fetch("/api/refresh", { method: "POST" });
      if (res.status === 409) {
        // Another collect is already in flight; keep polling for its result.
      } else if (!res.ok) {
        throw new Error(`refresh failed (${res.status})`);
      }
      // Poll until collected_at advances (or timeout).
      for (let i = 0; i < 20; i++) {
        await new Promise((r) => setTimeout(r, 1500));
        await load();
        if (snapshot.collected_at && snapshot.collected_at !== previous) break;
      }
    } finally {
      refreshBtn.disabled = false;
      refreshBtn.textContent = "Refresh data";
    }
  }

  sourceFilter.addEventListener("change", render);
  leagueFilter.addEventListener("change", render);
  windowFilter.addEventListener("change", render);
  refreshBtn.addEventListener("click", refresh);
  load().catch((err) => {
    statusMeta.textContent = `Failed to load: ${err.message}`;
    board.innerHTML = `<div class="empty">Could not load /api/splits</div>`;
  });
})();
