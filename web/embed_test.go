package web

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetsEmbeddedServesIndex(t *testing.T) {
	fsys, err := Assets(false)
	if err != nil {
		t.Fatalf("Assets(false): %v", err)
	}
	b, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !strings.Contains(string(b), "NFL Splitboard") {
		t.Error("embedded index.html missing brand text")
	}
	if !strings.Contains(string(b), `id="sortFilter"`) {
		t.Error("embedded index.html missing sort filter")
	}
	if !strings.Contains(string(b), `id="splitFilter"`) {
		t.Error("embedded index.html missing bet vs money gap filter")
	}
	if !strings.Contains(string(b), `src="/__livereload.js"`) {
		t.Error("embedded index.html missing live-reload script tag")
	}
}

func TestEmbeddedAppFlagsBetMoneyDivergence(t *testing.T) {
	fsys, err := Assets(false)
	if err != nil {
		t.Fatalf("Assets(false): %v", err)
	}
	b, err := fs.ReadFile(fsys, "app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	js := string(b)
	if !strings.Contains(js, "const DIVERGE_PTS = 10") {
		t.Error("app.js missing 10-pt bet vs money threshold")
	}
	if !strings.Contains(js, `class="diverge-badge"`) {
		t.Error("app.js missing divergence badge markup")
	}
	if !strings.Contains(js, `fill money${diverges ? " diverge" : ""}`) {
		t.Error("app.js does not recolor the money bar on divergence")
	}
	if !strings.Contains(js, `sortBy === "divergence"`) {
		t.Error("app.js missing sort by largest bet vs money gap")
	}
	if !strings.Contains(js, `splitView === "diverge"`) {
		t.Error("app.js missing filter to diverging matchups")
	}
	if !strings.Contains(js, "has-diverge") {
		t.Error("app.js missing matchup-level divergence class")
	}
	css, err := fs.ReadFile(fsys, "styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	if !strings.Contains(string(css), ".fill.money.diverge") {
		t.Error("styles.css missing money-bar diverge color")
	}
}

func TestDevAssetsReadsCurrentDiskContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "styles.css")
	if err := os.WriteFile(path, []byte("body{color:red}"), 0o644); err != nil {
		t.Fatal(err)
	}

	fsys := DevAssets(dir)
	if b, err := fs.ReadFile(fsys, "styles.css"); err != nil || string(b) != "body{color:red}" {
		t.Fatalf("first read: %q, %v", b, err)
	}

	// The whole point of dev mode: a later edit is visible without rebuilding.
	if err := os.WriteFile(path, []byte("body{color:blue}"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(fsys, "styles.css")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "body{color:blue}" {
		t.Errorf("got %q, want the edited contents", b)
	}
}
