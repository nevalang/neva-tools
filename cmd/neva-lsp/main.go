// neva-lsp starts the Neva Language Server Protocol implementation.
package main

import (
	"flag"

	"github.com/nevalang/neva-tools/internal/lsp"
)

func main() {
	debug := flag.Bool("debug", false, "run the server over TCP on localhost:6007")
	flag.Parse()
	if err := lsp.Run(*debug); err != nil {
		panic(err)
	}
}
