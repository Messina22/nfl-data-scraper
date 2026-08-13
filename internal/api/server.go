package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"nfl-data-scraper/internal/collect"
	"nfl-data-scraper/internal/store"
	"nfl-data-scraper/web"
)

// Server serves the splits dashboard and JSON API.
type Server struct {
	Store           *store.Store
	Addr            string
	RefreshInterval time.Duration

	mu         sync.Mutex
	collecting bool
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/splits", s.handleSplits)
	mux.HandleFunc("/api/sources", s.handleSources)
	mux.HandleFunc("/api/refresh", s.handleRefresh)

	static, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(static)))
	return mux
}

func (s *Server) Start(ctx context.Context) error {
	if err := s.Store.Load(); err != nil {
		log.Printf("load store: %v", err)
	}
	if len(s.Store.Latest().Games) == 0 {
		s.Refresh(ctx)
	}

	if s.RefreshInterval > 0 {
		go s.loop(ctx)
	}

	srv := &http.Server{Addr: s.Addr, Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("splits dashboard listening on http://%s", s.Addr)
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) loop(ctx context.Context) {
	t := time.NewTicker(s.RefreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Refresh(ctx)
		}
	}
}

func (s *Server) Refresh(ctx context.Context) {
	s.mu.Lock()
	if s.collecting {
		s.mu.Unlock()
		return
	}
	s.collecting = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.collecting = false
		s.mu.Unlock()
	}()

	snap := collect.Run(ctx, nil)
	if err := s.Store.Save(snap); err != nil {
		log.Printf("save snapshot: %v", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleSplits(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Store.Latest())
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	snap := s.Store.Latest()
	writeJSON(w, snap.Sources)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go s.Refresh(context.Background())
	writeJSON(w, map[string]any{"status": "refresh started"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
