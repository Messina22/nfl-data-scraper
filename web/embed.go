package web

import (
	"embed"
	"io/fs"
	"os"
)

// StaticFS holds the dashboard assets.
//
//go:embed static/*
var StaticFS embed.FS

// StaticDir is the on-disk source for dashboard assets in dev mode,
// relative to the process working directory (the repo root).
const StaticDir = "web/static"

// DevAssets serves dashboard assets straight from a directory on disk, so
// edits are visible without rebuilding the binary.
func DevAssets(dir string) fs.FS {
	return os.DirFS(dir)
}

// Assets returns the filesystem the dashboard is served from: live from disk
// in dev mode, otherwise the copy embedded at build time.
func Assets(dev bool) (fs.FS, error) {
	if dev {
		return DevAssets(StaticDir), nil
	}
	return fs.Sub(StaticFS, "static")
}
