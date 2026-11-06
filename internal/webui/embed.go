// Package webui embeds the browser assets.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed static
var assets embed.FS

// FS returns the static web root.
func FS() fs.FS {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
