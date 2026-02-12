package web

import "embed"

// DistFS embeds the built frontend assets.
//
//go:embed dist/*
var DistFS embed.FS
