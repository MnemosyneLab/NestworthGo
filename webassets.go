// Package webassets embeds the built frontend (frontend/dist) for
// cmd/nestworth. It is a tiny package at the repository root (rather than
// inside cmd/nestworth) only because Go's //go:embed directive cannot
// reference a path outside its own source file's directory, and frontend/
// is a sibling of cmd/ per the technical design's repository layout (Sec3),
// not a child of cmd/nestworth.
package webassets

import "embed"

// Dist contains the production frontend build (frontend/dist/**),
// produced by `pnpm run build` inside frontend/ or by the Taskfile's
// build:frontend task. frontend/dist/.gitkeep is committed so this
// embed succeeds on a clean checkout before Vite has produced the
// bundle. The Wails asset server auto-discovers index.html inside this
// tree regardless of the embed path prefix.
//
//go:embed all:frontend/dist
var Dist embed.FS
