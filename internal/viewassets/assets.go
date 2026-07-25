// Package viewassets provides the browser bundle used by every Neva View host.
package viewassets

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var files embed.FS

// FS returns the built visual-editor bundle. `make web-build` refreshes it.
func FS() (fs.FS, error) {
	return fs.Sub(files, "dist")
}
