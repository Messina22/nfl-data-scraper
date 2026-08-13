package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"nfl-data-scraper/internal/models"
)

// Store persists the latest splits snapshot to disk and memory.
type Store struct {
	mu   sync.RWMutex
	path string
	snap models.Snapshot
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string { return s.path }

func (s *Store) Latest() models.Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

func (s *Store) Save(snap models.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(snap)
}

// MergeSave merges incoming collection results with the in-memory snapshot,
// keeping last-good games for sources that failed this tick, then persists.
func (s *Store) MergeSave(incoming models.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(MergeSnapshots(s.snap, incoming))
}

func (s *Store) saveLocked(snap models.Snapshot) error {
	s.snap = snap

	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snap); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Load() error {
	if s.path == "" {
		return nil
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var snap models.Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return err
	}
	s.mu.Lock()
	s.snap = snap
	s.mu.Unlock()
	return nil
}
