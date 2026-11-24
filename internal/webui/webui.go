// Package webui embeds the compiled workbench UI.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built UI assets rooted at dist.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}

// Available reports whether a built UI is embedded.
func Available() bool {
	_, err := fs.Stat(FS(), "index.html")
	return err == nil
}
