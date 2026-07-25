package main

import (
	"io/fs"

	"github.com/nevalang/neva-lsp/internal/viewassets"
)

func embeddedWebDistFS() (fs.FS, error) {
	return viewassets.FS()
}
