package api

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// dirFingerprint summarizes every file under root by path, size, and modification
// time. Comparing two fingerprints detects edits, additions, and deletions in one
// string compare.
//
// Names starting with '.' or '_' are omitted, matching //go:embed static/* and
// skipping editor/OS junk. Nested directories with those prefixes are not
// entered (fs.SkipDir). The walk root itself is always entered.
//
// Walk and Info errors omit the affected file rather than failing the whole
// fingerprint. A consistently missing root yields "" (stable). A directory that
// becomes unreadable drops entries and will look like a change.
func dirFingerprint(root string) string {
	var sb strings.Builder
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && isEmbedExcludedName(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if isEmbedExcludedName(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		// Same-size writes within a single filesystem timestamp tick cannot be distinguished.
		fmt.Fprintf(&sb, "%s:%d:%d\n", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return sb.String()
}

func isEmbedExcludedName(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}
