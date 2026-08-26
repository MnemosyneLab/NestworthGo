// Package webassets embeds the built frontend (frontend/dist) for
// cmd/nestworth-desktop. It is a tiny package at the repository root
// (rather than inside cmd/nestworth-desktop) only because Go's //go:embed
// directive cannot reference a path outside its own source file's
// directory, and frontend/ is a sibling of cmd/ per the technical
// design's repository layout (Sec3), not a child of
// cmd/nestworth-desktop.
package webassets

import "embed"

// Dist contains the production frontend build (frontend/dist/**),
// produced by `npm run build` inside frontend/ or by the Taskfile's
// build:frontend task. The Wails asset server auto-discovers index.html
// inside this tree regardless of the embed path prefix.
//
//go:embed all:frontend/dist
var Dist embed.FS
