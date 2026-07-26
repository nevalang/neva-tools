package lsp

import (
	"time"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

const unsavedDiagnosticsDelay = 150 * time.Millisecond

func (s *Server) TextDocumentDidOpen(
	glspCtx *glsp.Context,
	params *protocol.DidOpenTextDocumentParams,
) error {
	s.activeFileMutex.Lock()
	s.activeFile = params.TextDocument.URI
	s.activeFileMutex.Unlock()
	s.setOpenDocument(params.TextDocument.URI, params.TextDocument.Text)
	if glspCtx != nil {
		s.scheduleIndexAndNotifyProblems(glspCtx.Notify)
	}
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
	if glspCtx != nil {
		s.scheduleIndexAndNotifyProblems(glspCtx.Notify)
	}
	return nil
}

func (s *Server) TextDocumentDidSave(
	glspCtx *glsp.Context,
	params *protocol.DidSaveTextDocumentParams,
) error {
	if params.Text != nil {
		s.setOpenDocument(params.TextDocument.URI, *params.Text)
	}
	s.cancelScheduledIndex()
	if s.logger != nil {
		s.logger.Info("TextDocumentDidSave")
	}
	if glspCtx == nil {
		return nil
	}
	return s.indexAndNotifyProblems(glspCtx.Notify)
}

func (s *Server) TextDocumentDidClose(
	glspCtx *glsp.Context,
	params *protocol.DidCloseTextDocumentParams,
) error {
	s.deleteOpenDocument(params.TextDocument.URI)
	if glspCtx != nil {
		s.scheduleIndexAndNotifyProblems(glspCtx.Notify)
	}
	return nil
}

func (s *Server) scheduleIndexAndNotifyProblems(notify glsp.NotifyFunc) {
	s.diagnosticsTimerMutex.Lock()
	defer s.diagnosticsTimerMutex.Unlock()

	if s.diagnosticsTimer != nil {
		s.diagnosticsTimer.Stop()
	}
	s.diagnosticsTimer = time.AfterFunc(unsavedDiagnosticsDelay, func() {
		if err := s.indexAndNotifyProblems(notify); err != nil && s.logger != nil {
			s.logger.Error("index unsaved document", "err", err)
		}
	})
}

func (s *Server) cancelScheduledIndex() {
	s.diagnosticsTimerMutex.Lock()
	defer s.diagnosticsTimerMutex.Unlock()
	if s.diagnosticsTimer != nil {
		s.diagnosticsTimer.Stop()
		s.diagnosticsTimer = nil
	}
}
