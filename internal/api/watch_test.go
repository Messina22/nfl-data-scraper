package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirFingerprintStableWithoutChanges(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if first, second := dirFingerprint(dir), dirFingerprint(dir); first != second {
		t.Errorf("fingerprint changed with no edit:\n%q\n%q", first, second)
	}
}

func TestDirFingerprintChangesOnEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "styles.css")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := dirFingerprint(dir)

	// Different length, so the check holds even at coarse modtime resolution.
	if err := os.WriteFile(path, []byte("bbbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	if second := dirFingerprint(dir); first == second {
		t.Error("fingerprint did not change after edit")
	}
}

func TestDirFingerprintDetectsNewFileInSubdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := dirFingerprint(dir)

	sub := filepath.Join(dir, "img")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "logo.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if second := dirFingerprint(dir); first == second {
		t.Error("fingerprint did not change after adding a file in a subdirectory")
	}
}

func TestDirFingerprintMissingDirIsEmpty(t *testing.T) {
	if got := dirFingerprint(filepath.Join(t.TempDir(), "nope")); got != "" {
		t.Errorf("got %q, want empty string for a missing directory", got)
	}
}
