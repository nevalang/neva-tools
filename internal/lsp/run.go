package lsp

import (
	"github.com/nevalang/neva/pkg/indexer"
	"github.com/tliron/commonlog"
	_ "github.com/tliron/commonlog/simple"
	"github.com/tliron/glsp/server"
)

// Run starts the Neva language server over stdio, or TCP in debug mode.
func Run(debug bool) error {
	const serverName = "neva"

	loglvl := 1
	if debug {
		loglvl = 2
	}

	commonlog.Configure(loglvl, nil)
	logger := commonlog.GetLoggerf("%s.server", serverName)

	indexer := indexer.MustNewDefault(logger)

	handler := BuildHandler(logger, serverName, indexer)

	srv := server.NewServer(
		handler,
		serverName,
		debug,
	)

	if debug {
		return srv.RunTCP("localhost:6007")
	}
	return srv.RunStdio()
}
