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

  function matchupKey(g) {
    const league = (g.league || "").toLowerCase();
    const away = (g.away_abbr || g.away_team || "").toLowerCase();
    const home = (g.home_abbr || g.home_team || "").toLowerCase();
    const day = g.start_time ? g.start_time.slice(0, 10) : "na";
    return `${league}|${day}|${away}|${home}`;
  }

  function normalizeTeam(name) {
    return (name || "").toLowerCase().replace(/[^a-z0-9]/g, "");
  }

  function teamsMatch(a, b) {
    const x = normalizeTeam(a);
    const y = normalizeTeam(b);
    if (!x || !y) return false;
    return x === y || x.includes(y) || y.includes(x);
  }

  function leaguesMatch(a, b) {
    const x = (a || "").toLowerCase();
    const y = (b || "").toLowerCase();
    if (!x || !y) return true;
    return x === y;
  }

  function groupGames(games) {
    const groups = [];
    for (const g of games) {
      let found = groups.find((grp) => {
        return (
          leaguesMatch(grp.league, g.league) &&
          teamsMatch(grp.away, g.away_team) &&
          teamsMatch(grp.home, g.home_team)
        );
      });
      if (!found) {
        found = {
          key: matchupKey(g),
          away: g.away_team,
          home: g.home_team,
          league: g.league || "",
          start: g.start_time,
          reports: [],
        };
        groups.push(found);
      }
      found.reports.push(g);
      if (!found.start && g.start_time) found.start = g.start_time;
      if (!found.league && g.league) found.league = g.league;
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

  function marketBlock(market, insight) {
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
              <span>${escapeHtml(side.label || side.side)}</span>
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
      games = games.filter((g) => (g.league || "").toLowerCase() === league.toLowerCase());
    }
    games = games.filter((g) => inWindow(g.start_time, days));

    const groups = groupGames(games);
    sourcePills.innerHTML = (snapshot.sources || [])
      .map((s) => {
        const cls = s.ok ? "pill ok" : "pill";
        const detail = s.ok ? `${s.games} games` : "error";
        return `<span class="${cls}" title="${escapeHtml(s.error || "")}"><span class="dot"></span>${escapeHtml(s.name)} · ${detail}</span>`;
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
                  <span>${escapeHtml(g.source_name)}${g.book ? ` · ${escapeHtml(g.book)}` : ""}</span>
                  <span>${g.num_bets != null ? `${g.num_bets.toLocaleString()} bets tracked` : ""}</span>
                </div>
                <div class="markets">
                  ${marketBlock(byMarket.spread, insightForMarket(insights, "spread"))}
                  ${marketBlock(byMarket.moneyline, insightForMarket(insights, "moneyline"))}
                  ${marketBlock(byMarket.total, insightForMarket(insights, "total"))}
                </div>
              </div>`;
          })
          .join("");
        return `
          <section class="matchup" style="animation-delay:${Math.min(idx * 0.04, 0.4)}s">
            <div class="matchup-head">
              <h2 class="matchup-title">${escapeHtml(grp.away)} <span style="opacity:.45">@</span> ${escapeHtml(grp.home)}</h2>
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
      const name = (g.league || "").trim();
      if (!name) continue;
      const key = name.toLowerCase();
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
