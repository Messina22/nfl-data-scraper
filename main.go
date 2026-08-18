package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nfl-data-scraper/internal/api"
	"nfl-data-scraper/internal/collect"
	"nfl-data-scraper/internal/store"
)

func main() {
	addr := flag.String("addr", envOr("SPLITS_ADDR", "127.0.0.1:8080"), "HTTP listen address")
	dataPath := flag.String("data", envOr("SPLITS_DATA", "data/splits.json"), "snapshot JSON path")
	refresh := flag.Duration("refresh", 15*time.Minute, "auto-refresh interval (0 to disable)")
	collectOnly := flag.Bool("collect-only", false, "collect once and write snapshot, then exit")
	dev := flag.Bool("dev", false, "serve assets from disk and enable live reload")
	flag.Parse()

	st := store.New(*dataPath)

	if *collectOnly {
		if err := st.Load(); err != nil {
			log.Printf("load store: %v", err)
		}
		snap := collect.Run(context.Background(), nil)
		if err := st.MergeSave(snap); err != nil {
			log.Fatal(err)
		}
		merged := st.Latest()
		log.Printf("wrote %d game reports from %d sources to %s", len(merged.Games), len(merged.Sources), *dataPath)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &api.Server{
		Store:           st,
		Addr:            *addr,
		RefreshInterval: *refresh,
		Dev:             *dev,
	}
	if err := srv.Start(ctx); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
