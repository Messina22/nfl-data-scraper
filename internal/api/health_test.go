package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"nfl-data-scraper/internal/models"
)

func decodeHealth(t *testing.T, recBody []byte) healthResponse {
	t.Helper()
	var body healthResponse
	if err := json.Unmarshal(recBody, &body); err != nil {
		t.Fatalf("decode health: %v\n%s", err, recBody)
	}
	return body
}

func TestHealthEmptySnapshotIsLive(t *testing.T) {
	rec := get(newTestServer(t, false).Handler(), "/api/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body := decodeHealth(t, rec.Body.Bytes())
	if !body.OK {
		t.Error("empty snapshot should still be live (ok: true)")
	}
	if body.Failed != 0 {
		t.Errorf("failed = %d, want 0", body.Failed)
	}
	if body.Sources == nil {
		t.Fatal("sources must be [] not null")
	}
	if len(body.Sources) != 0 {
		t.Errorf("sources len = %d, want 0", len(body.Sources))
	}
}

func TestHealthReportsPerSourceStatus(t *testing.T) {
	s := newTestServer(t, false)
	now := time.Date(2026, 8, 18, 19, 0, 0, 0, time.UTC)
	if err := s.Store.Save(models.Snapshot{
		CollectedAt: now,
		Sources: []models.SourceStatus{
			{ID: "vsin-dk", Name: "VSiN (DraftKings)", OK: true, Games: 8, FetchedAt: now},
			{ID: "covers-consensus", Name: "Covers Consensus", OK: false, Error: "no matchup split rows", Games: 0, FetchedAt: now},
		},
	}); err != nil {
		t.Fatal(err)
	}

	rec := get(s.Handler(), "/api/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	body := decodeHealth(t, rec.Body.Bytes())
	if !body.OK {
		t.Error("one expected source failure should not fail overall health")
	}
	if body.Failed != 1 {
		t.Errorf("failed = %d, want 1", body.Failed)
	}
	if !body.CollectedAt.Equal(now) {
		t.Errorf("collected_at = %v, want %v", body.CollectedAt, now)
	}
	if len(body.Sources) != 2 {
		t.Fatalf("sources len = %d, want 2", len(body.Sources))
	}

	byID := map[string]models.SourceStatus{}
	for _, src := range body.Sources {
		byID[src.ID] = src
	}
	covers := byID["covers-consensus"]
	if covers.OK {
		t.Error("covers-consensus should report ok: false")
	}
	if covers.Error != "no matchup split rows" {
		t.Errorf("covers error = %q", covers.Error)
	}
	vsin := byID["vsin-dk"]
	if !vsin.OK || vsin.Games != 8 {
		t.Errorf("vsin-dk = %+v, want ok with 8 games", vsin)
	}
}

func TestHealthAllSourcesFailed(t *testing.T) {
	s := newTestServer(t, false)
	if err := s.Store.Save(models.Snapshot{
		CollectedAt: time.Now().UTC(),
		Sources: []models.SourceStatus{
			{ID: "vsin-dk", Name: "VSiN", OK: false, Error: "timeout"},
			{ID: "dk-network", Name: "DraftKings Network", OK: false, Error: "403"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	rec := get(s.Handler(), "/api/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (liveness must not restart on collector failure)", rec.Code)
	}
	body := decodeHealth(t, rec.Body.Bytes())
	if body.OK {
		t.Error("ok should be false when every collector failed")
	}
	if body.Failed != 2 {
		t.Errorf("failed = %d, want 2", body.Failed)
	}
}

func TestHealthFromSnapshotNilSources(t *testing.T) {
	got := healthFromSnapshot(models.Snapshot{})
	if !got.OK {
		t.Error("nil sources should be live")
	}
	if got.Sources == nil {
		t.Fatal("sources must be empty slice, not nil")
	}
}
