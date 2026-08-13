package collect

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"nfl-data-scraper/internal/models"
	"nfl-data-scraper/internal/sources"
)

// Run collects splits from every registered source concurrently.
func Run(ctx context.Context, srcs []sources.Source) models.Snapshot {
	if srcs == nil {
		srcs = sources.Registry()
	}

	type result struct {
		status models.SourceStatus
		games   []models.GameSplits
	}

	ch := make(chan result, len(srcs))
	var wg sync.WaitGroup
	for _, src := range srcs {
		wg.Add(1)
		go func(s sources.Source) {
			defer wg.Done()
			started := time.Now().UTC()
			games, err := s.Collect(ctx)
			st := models.SourceStatus{
				ID:        s.ID(),
				Name:      s.Name(),
				FetchedAt: started,
			}
			if err != nil {
				st.OK = false
				st.Error = err.Error()
				log.Printf("source %s: %v", s.ID(), err)
			} else {
				st.OK = true
				st.Games = len(games)
				log.Printf("source %s: %d games", s.ID(), len(games))
			}
			ch <- result{status: st, games: games}
		}(src)
	}
	wg.Wait()
	close(ch)

	snap := models.Snapshot{CollectedAt: time.Now().UTC()}
	for r := range ch {
		snap.Sources = append(snap.Sources, r.status)
		snap.Games = append(snap.Games, r.games...)
	}

	sort.Slice(snap.Sources, func(i, j int) bool {
		return snap.Sources[i].ID < snap.Sources[j].ID
	})
	sort.SliceStable(snap.Games, func(i, j int) bool {
		ai, aj := snap.Games[i], snap.Games[j]
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
	return snap
}
