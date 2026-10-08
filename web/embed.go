// Package web embeds the built SPA (web/dist, produced by `vite build`) so
// the Go binary can serve it directly.
package web

import "embed"

// DistFS holds web/dist, everything in it including dotfiles. The committed
// dist/.gitkeep keeps the embed valid before any frontend build, when it
// holds no app.
//
//go:embed all:dist
var DistFS embed.FS
