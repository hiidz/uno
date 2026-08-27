// Package web embeds the built SPA (web/dist, produced by `vite build`) so
// the Go binary can serve it directly
package web

import "embed"

//go:embed all:dist
var DistFS embed.FS
