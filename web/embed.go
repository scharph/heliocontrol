// Package web embeds the compiled Angular UI (built into web/dist/browser).
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist/browser
var dist embed.FS

// FS returns the UI files rooted at the app shell (index.html).
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist/browser")
	if err != nil {
		panic(err)
	}
	return sub
}
