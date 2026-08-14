package sources

import (
	"context"
	"os"

	"nfl-data-scraper/internal/models"
)

// Source collects NFL betting splits from one reported provider.
type Source interface {
	ID() string
	Name() string
	Collect(ctx context.Context) ([]models.GameSplits, error)
}

// Registry returns the built-in collectors we can retrieve today.
func Registry() []Source {
	return []Source{
		NewVSiN("DK", "DraftKings"),
		NewVSiN("CIRCA", "Circa"),
		NewActionNetwork(os.Getenv("ACTION_NETWORK_COOKIE")),
		NewCoversConsensus(),
	}
}
