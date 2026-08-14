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
