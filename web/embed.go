package web

import "embed"

// StaticFS holds the dashboard assets.
//
//go:embed static/*
var StaticFS embed.FS
