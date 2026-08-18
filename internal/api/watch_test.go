package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestDirFingerprintIgnoresDotfiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := dirFingerprint(dir)

	dotfile := filepath.Join(dir, ".styles.css.swp")
	if err := os.WriteFile(dotfile, []byte("swap"), 0o644); err != nil {
		t.Fatal(err)
	}
	if second := dirFingerprint(dir); first != second {
		t.Errorf("fingerprint changed after adding a dotfile:\n%q\n%q", first, second)
	}

	// Editing the dotfile must not change the fingerprint either.
	if err := os.WriteFile(dotfile, []byte("swap edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if third := dirFingerprint(dir); first != third {
		t.Errorf("fingerprint changed after editing a dotfile:\n%q\n%q", first, third)
	}
}

func TestDirFingerprintIgnoresUnderscorePrefixed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := dirFingerprint(dir)

	if err := os.WriteFile(filepath.Join(dir, "_draft.css"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if second := dirFingerprint(dir); first != second {
		t.Errorf("fingerprint changed after adding an underscore-prefixed file:\n%q\n%q", first, second)
	}

	sub := filepath.Join(dir, "_notes")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "x.css"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if third := dirFingerprint(dir); first != third {
		t.Errorf("fingerprint changed after adding an underscore-prefixed directory:\n%q\n%q", first, third)
	}
}

func TestDirFingerprintSkipsHiddenDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := dirFingerprint(dir)

	hidden := filepath.Join(dir, ".cache")
	if err := os.Mkdir(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "foo.css"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if second := dirFingerprint(dir); first != second {
		t.Errorf("fingerprint changed after adding a hidden directory:\n%q\n%q", first, second)
	}
}

func TestDirFingerprintDetectsDeletion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.js")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "styles.css"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := dirFingerprint(dir)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if second := dirFingerprint(dir); first == second {
		t.Error("fingerprint did not change after deletion")
	}
}

func TestDirFingerprintMissingDirIsEmpty(t *testing.T) {
	if got := dirFingerprint(filepath.Join(t.TempDir(), "nope")); got != "" {
		t.Errorf("got %q, want empty string for a missing directory", got)
	}
}

func TestDirFingerprintChangesOnSameSizeEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "styles.css")
	if err := os.WriteFile(path, []byte("#ff0000"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := dirFingerprint(dir)

	// Same byte count, so only the modification time can distinguish these.
	if err := os.WriteFile(path, []byte("#00ff00"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Force a distinct mtime so the test does not depend on filesystem
	// timestamp resolution.
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}

	if second := dirFingerprint(dir); first == second {
		t.Error("fingerprint did not change after a same-size edit")
	}
}
