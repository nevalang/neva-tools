package main

import (
	"embed"
	"io/fs"
)

//go:embed assets/*
var files embed.FS

// FS returns the built visual-editor bundle. `make web-build` refreshes it.
func embeddedUIFS() (fs.FS, error) {
	return fs.Sub(files, "assets")
}
