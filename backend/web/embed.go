// Package web embeds the built H5 frontend so the server binary is
// self-contained (same pattern as omnibot: go:embed of the built dist).
//
// The real build comes from frontend/h5 — its Vite config outputs
// directly to this dist/ directory. The checked-in index.html is only
// a placeholder so the embed compiles before the first frontend build.
package web

import "embed"

//go:embed all:dist
var FS embed.FS
