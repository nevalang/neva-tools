package main

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) TextDocumentDidOpen(
	glspCtx *glsp.Context,
	params *protocol.DidOpenTextDocumentParams,
) error {
	s.activeFileMutex.Lock()
	s.activeFile = params.TextDocument.URI
	s.activeFileMutex.Unlock()
	s.setOpenDocument(params.TextDocument.URI, params.TextDocument.Text)
	return nil
}

func (s *Server) TextDocumentDidChange(
	glspCtx *glsp.Context,
	params *protocol.DidChangeTextDocumentParams,
) error {
	s.activeFileMutex.Lock()
	s.activeFile = params.TextDocument.URI
	s.activeFileMutex.Unlock()
	s.applyOpenDocumentChanges(params.TextDocument.URI, params.ContentChanges)
	return nil
}

func (s *Server) TextDocumentDidSave(
	glspCtx *glsp.Context,
	params *protocol.DidSaveTextDocumentParams,
) error {
	if params.Text != nil {
		s.setOpenDocument(params.TextDocument.URI, *params.Text)
	}
	s.logger.Info("TextDocumentDidSave")
	return s.indexAndNotifyProblems(glspCtx.Notify)
}

func (s *Server) TextDocumentDidClose(
	glspCtx *glsp.Context,
	params *protocol.DidCloseTextDocumentParams,
) error {
	s.deleteOpenDocument(params.TextDocument.URI)
	return nil
}
