package api

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// dirFingerprint summarizes every file under root by path, size, and modification
// time. Comparing two fingerprints detects edits, additions, and deletions in one
// string compare. Walk errors are ignored: a missing or unreadable directory
// yields a stable value rather than spurious reloads.
func dirFingerprint(root string) string {
	var sb strings.Builder
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// Skip dotfiles (editor swap files, .DS_Store) so they don't trigger
		// spurious reloads while editing. Production also excludes them, since
		// //go:embed static/* skips '.'- and '_'-prefixed files.
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		// Note: same-size writes within a single filesystem timestamp tick cannot be distinguished.
		fmt.Fprintf(&sb, "%s:%d:%d\n", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return sb.String()
}
