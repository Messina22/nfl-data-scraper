package store

import (
	"strings"
	"testing"
	"time"

	"nfl-data-scraper/internal/models"
)

func TestMergeSnapshotsKeepsLastGoodOnFailure(t *testing.T) {
	prev := models.Snapshot{
		CollectedAt: time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
		Sources: []models.SourceStatus{
			{ID: "vsin-dk", Name: "VSiN DK", OK: true, Games: 1},
			{ID: "vsin-circa", Name: "VSiN Circa", OK: true, Games: 1},
		},
		Games: []models.GameSplits{
			{SourceID: "vsin-dk", AwayTeam: "A", HomeTeam: "B"},
			{SourceID: "vsin-circa", AwayTeam: "C", HomeTeam: "D"},
		},
	}
	incoming := models.Snapshot{
		CollectedAt: time.Date(2026, 8, 13, 12, 15, 0, 0, time.UTC),
		Sources: []models.SourceStatus{
			{ID: "vsin-dk", Name: "VSiN DK", OK: false, Error: "timeout", Games: 0},
			{ID: "vsin-circa", Name: "VSiN Circa", OK: true, Games: 1},
		},
		Games: []models.GameSplits{
			{SourceID: "vsin-circa", AwayTeam: "C2", HomeTeam: "D2"},
		},
	}

	got := MergeSnapshots(prev, incoming)
	if !got.CollectedAt.Equal(incoming.CollectedAt) {
		t.Fatalf("collected_at = %v, want incoming", got.CollectedAt)
	}
	if len(got.Games) != 2 {
		t.Fatalf("games = %d, want 2 (kept dk + new circa)", len(got.Games))
	}

	bySource := map[string]models.GameSplits{}
	for _, g := range got.Games {
		bySource[g.SourceID] = g
	}
	if bySource["vsin-dk"].AwayTeam != "A" {
		t.Fatalf("dk not retained: %+v", bySource["vsin-dk"])
	}
	if bySource["vsin-circa"].AwayTeam != "C2" {
		t.Fatalf("circa not replaced: %+v", bySource["vsin-circa"])
	}

	var dkStatus models.SourceStatus
	for _, st := range got.Sources {
		if st.ID == "vsin-dk" {
			dkStatus = st
		}
	}
	if dkStatus.OK || dkStatus.Games != 1 {
		t.Fatalf("dk status = %+v, want OK=false Games=1", dkStatus)
	}
	if dkStatus.Error == "" || !strings.Contains(dkStatus.Error, "retaining last good data") {
		t.Fatalf("dk error should note retention: %q", dkStatus.Error)
	}
}

func TestMergeSnapshotsReplacesOnRecovery(t *testing.T) {
	prev := models.Snapshot{
		Sources: []models.SourceStatus{
			{ID: "vsin-dk", OK: false, Error: "timeout", Games: 1},
		},
		Games: []models.GameSplits{
			{SourceID: "vsin-dk", AwayTeam: "Old", HomeTeam: "Data"},
		},
	}
	incoming := models.Snapshot{
		CollectedAt: time.Date(2026, 8, 13, 13, 0, 0, 0, time.UTC),
		Sources: []models.SourceStatus{
			{ID: "vsin-dk", OK: true, Games: 1},
		},
		Games: []models.GameSplits{
			{SourceID: "vsin-dk", AwayTeam: "Fresh", HomeTeam: "Data"},
		},
	}

	got := MergeSnapshots(prev, incoming)
	if len(got.Games) != 1 || got.Games[0].AwayTeam != "Fresh" {
		t.Fatalf("expected fresh games, got %+v", got.Games)
	}
	if !got.Sources[0].OK {
		t.Fatalf("expected OK after recovery")
	}
}

func TestMergeSnapshotsPartialSourceKeepsOtherLeagues(t *testing.T) {
	prev := models.Snapshot{
		Sources: []models.SourceStatus{
			{ID: "dk-network", Name: "DraftKings Network", OK: true, Games: 2},
		},
		Games: []models.GameSplits{
			{SourceID: "dk-network", League: "MLB", AwayTeam: "OldMLBAway", HomeTeam: "OldMLBHome"},
			{SourceID: "dk-network", League: "NFL", AwayTeam: "OldNFLAway", HomeTeam: "OldNFLHome"},
		},
	}
	incoming := models.Snapshot{
		CollectedAt: time.Date(2026, 8, 14, 5, 0, 0, 0, time.UTC),
		Sources: []models.SourceStatus{
			{ID: "dk-network", Name: "DraftKings Network", OK: false, Error: "NFL: 403", Games: 1},
		},
		Games: []models.GameSplits{
			{SourceID: "dk-network", League: "NFL", AwayTeam: "NewNFLAway", HomeTeam: "NewNFLHome"},
		},
	}

	got := MergeSnapshots(prev, incoming)
	if len(got.Games) != 2 {
		t.Fatalf("games = %d, want 2 (new NFL + last-good MLB)", len(got.Games))
	}
	byLeague := map[string]models.GameSplits{}
	for _, g := range got.Games {
		byLeague[g.League] = g
	}
	if byLeague["MLB"].AwayTeam != "OldMLBAway" {
		t.Fatalf("MLB not retained: %+v", byLeague["MLB"])
	}
	if byLeague["NFL"].AwayTeam != "NewNFLAway" {
		t.Fatalf("NFL not replaced: %+v", byLeague["NFL"])
	}
	if got.Sources[0].OK || got.Sources[0].Games != 2 {
		t.Fatalf("status = %+v, want OK=false Games=2", got.Sources[0])
	}
	if !strings.Contains(got.Sources[0].Error, "retaining last good data") {
		t.Fatalf("error should note retention: %q", got.Sources[0].Error)
	}
}
