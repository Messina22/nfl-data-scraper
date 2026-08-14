package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

	// Dev serves assets from disk and enables live reload. Never set in production.
	Dev bool

	bootID string

	mu         sync.Mutex
	collecting bool
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/splits", s.handleSplits)
	mux.HandleFunc("/api/sources", s.handleSources)
	mux.HandleFunc("/api/refresh", s.handleRefresh)

	if s.Dev {
		if s.bootID == "" {
			s.bootID = newBootID()
		}
		mux.HandleFunc("/api/livereload", s.handleLiveReload)
		mux.HandleFunc("/__livereload.js", s.handleLiveReloadScript)
	}

	assets, err := web.Assets(s.Dev)
	if err != nil {
		log.Fatal(err)
	}
	var files http.Handler = http.FileServer(http.FS(assets))
	if s.Dev {
		files = noCache(files)
	}
	mux.Handle("/", files)
	return mux
}

func (s *Server) Start(ctx context.Context) error {
	if err := s.Store.Load(); err != nil {
		log.Printf("load store: %v", err)
	}
	if s.Dev {
		if _, err := os.Stat(filepath.Join(web.StaticDir, "index.html")); err != nil {
			log.Printf("dev mode: cannot read %s/index.html — run from the repo root: %v", web.StaticDir, err)
		}
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
	if !s.beginCollect() {
		return
	}
	defer s.endCollect()

	snap := collect.Run(ctx, nil)
	if err := s.Store.MergeSave(snap); err != nil {
		log.Printf("save snapshot: %v", err)
	}
}

func (s *Server) beginCollect() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.collecting {
		return false
	}
	s.collecting = true
	return true
}

func (s *Server) endCollect() {
	s.mu.Lock()
	s.collecting = false
	s.mu.Unlock()
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
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.beginCollect() {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{"status": "already_running"})
		return
	}
	go func() {
		defer s.endCollect()
		snap := collect.Run(context.Background(), nil)
		if err := s.Store.MergeSave(snap); err != nil {
			log.Printf("save snapshot: %v", err)
		}
	}()
	writeJSON(w, map[string]any{"status": "refresh started"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
