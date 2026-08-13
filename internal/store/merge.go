package store

import (
	"sort"
	"time"

	"nfl-data-scraper/internal/models"
)

// MergeSnapshots keeps last-good games per source when a source fails.
// Successful sources (OK==true) replace that source's games with the new batch.
// CollectedAt always comes from incoming so refresh clients can detect completion.
func MergeSnapshots(prev, incoming models.Snapshot) models.Snapshot {
	prevGames := map[string][]models.GameSplits{}
	for _, g := range prev.Games {
		prevGames[g.SourceID] = append(prevGames[g.SourceID], g)
	}
	prevStatus := map[string]models.SourceStatus{}
	for _, st := range prev.Sources {
		prevStatus[st.ID] = st
	}

	incomingGames := map[string][]models.GameSplits{}
	for _, g := range incoming.Games {
		incomingGames[g.SourceID] = append(incomingGames[g.SourceID], g)
	}

	out := models.Snapshot{CollectedAt: incoming.CollectedAt}
	if out.CollectedAt.IsZero() {
		out.CollectedAt = time.Now().UTC()
	}

	seen := map[string]bool{}
	for _, st := range incoming.Sources {
		seen[st.ID] = true
		if st.OK {
			games := incomingGames[st.ID]
			st.Games = len(games)
			out.Sources = append(out.Sources, st)
			out.Games = append(out.Games, games...)
			continue
		}
		// Failed collect: retain previous games for this source when present.
		kept := prevGames[st.ID]
		st.Games = len(kept)
		if len(kept) > 0 && st.Error != "" {
			st.Error = st.Error + "; retaining last good data"
		}
		out.Sources = append(out.Sources, st)
		out.Games = append(out.Games, kept...)
	}

	// Preserve sources that were not attempted this tick (e.g. removed temporarily).
	for id, st := range prevStatus {
		if seen[id] {
			continue
		}
		kept := prevGames[id]
		st.Games = len(kept)
		out.Sources = append(out.Sources, st)
		out.Games = append(out.Games, kept...)
	}

	sort.Slice(out.Sources, func(i, j int) bool {
		return out.Sources[i].ID < out.Sources[j].ID
	})
	sort.SliceStable(out.Games, func(i, j int) bool {
		ai, aj := out.Games[i], out.Games[j]
		si, sj := "", ""
		if ai.StartTime != nil {
			si = ai.StartTime.Format(time.RFC3339)
		}
		if aj.StartTime != nil {
			sj = aj.StartTime.Format(time.RFC3339)
		}
		if si != sj {
			return si < sj
		}
		if ai.AwayTeam != aj.AwayTeam {
			return ai.AwayTeam < aj.AwayTeam
		}
		if ai.HomeTeam != aj.HomeTeam {
			return ai.HomeTeam < aj.HomeTeam
		}
		return ai.SourceID < aj.SourceID
	})
	return out
}
