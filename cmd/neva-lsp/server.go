package main

import (
	"context"
	"math"
	"path/filepath"
	"sync"
	"time"

	"github.com/tliron/commonlog"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	src "github.com/nevalang/neva/pkg/ast"
	"github.com/nevalang/neva/pkg/core"
	"github.com/nevalang/neva/pkg/indexer"
)

//nolint:govet // fieldalignment: preserve layout for readability.
type Server struct {
	workspacePath string
	name, version string

	handler *Handler
	logger  commonlog.Logger
	indexer indexer.Indexer

	indexMutex *sync.Mutex
	index      *src.Build

	problemsMutex *sync.Mutex
	problemFiles  map[string]struct{} // we only need to store file urls but not their problems

	activeFile      string
	activeFileMutex *sync.Mutex

	openDocsMutex *sync.Mutex
	openDocs      map[string]string
}

// getBuild returns the latest indexed build snapshot when available.
// Read locking is required because indexing updates s.index concurrently with LSP request handling.
func (s *Server) getBuild() (*src.Build, bool) {
	s.indexMutex.Lock()
	defer s.indexMutex.Unlock()
	if s.index == nil {
		return nil, false
	}
	return s.index, true
}

// setBuild replaces the indexed build snapshot.
// Writers share the same lock with getBuild to avoid data races on the build pointer.
func (s *Server) setBuild(build src.Build) {
	s.indexMutex.Lock()
	s.index = &build
	s.indexMutex.Unlock()
}

// indexAndNotifyProblems does full scan of the workspace
// and sends diagnostics if there are any problems
func (s *Server) indexAndNotifyProblems(notify glsp.NotifyFunc) error {
	build, found, compilerErr := s.indexer.FullScan(
		context.Background(),
		s.workspacePath,
	)
	if !found {
		return nil
	}

	s.setBuild(build)

	if compilerErr == nil {
		// clear problems
		s.problemsMutex.Lock()
		for uri := range s.problemFiles {
			notify(
				protocol.ServerTextDocumentPublishDiagnostics,
				protocol.PublishDiagnosticsParams{
					URI:         uri,
					Diagnostics: []protocol.Diagnostic{},
				},
			)
		}
		s.problemFiles = make(map[string]struct{})
		s.logger.Info("full index without problems, sent empty diagnostics")
		s.problemsMutex.Unlock()
		return nil
	}

	// remember problem and send diagnostic
	s.problemsMutex.Lock()
	uri := s.workspacePath
	if compilerErr.Meta != nil {
		uri = filepath.Join(s.workspacePath, compilerErr.Meta.Location.String())
	}
	s.problemFiles[uri] = struct{}{}
	notify(
		protocol.ServerTextDocumentPublishDiagnostics,
		s.createDiagnostics(*compilerErr, uri),
	)
	s.logger.Info("diagnostic sent:", "err", compilerErr)
	s.problemsMutex.Unlock()

	return nil
}

func (s *Server) createDiagnostics(
	indexerErr indexer.Error,
	uri string,
) protocol.PublishDiagnosticsParams {
	startStopRange := diagnosticRange(indexerErr.Meta)

	return protocol.PublishDiagnosticsParams{
		URI: uri, // uri must be full path to the file, make sure all compiler errors include full location
		Diagnostics: []protocol.Diagnostic{
			{
				Range:    startStopRange,
				Severity: ptr(protocol.DiagnosticSeverityError),
				Source:   ptr("compiler"),
				Message:  indexerErr.Message, // we don't use Error() because it will duplicate location
				Data:     time.Now(),
			},
		},
	}
}

func diagnosticRange(meta *core.Meta) protocol.Range {
	defaultRange := protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End:   protocol.Position{Line: 0, Character: 1},
	}
	if meta == nil {
		return defaultRange
	}

	start, startOK := diagnosticStartPosition(meta.Start)
	if !startOK {
		return defaultRange
	}

	end, endOK := diagnosticStopPosition(meta.Start, meta.Stop)
	if !endOK {
		end = nextCharacterPosition(start)
	}

	if end.Line < start.Line || (end.Line == start.Line && end.Character < start.Character) {
		end = nextCharacterPosition(start)
	}

	return protocol.Range{Start: start, End: end}
}

func diagnosticStartPosition(pos core.Position) (protocol.Position, bool) {
	if pos.Line <= 0 {
		return protocol.Position{}, false
	}

	return protocol.Position{
		Line:      clampToUint32(pos.Line - 1),
		Character: clampToUint32(pos.Column),
	}, true
}

func diagnosticStopPosition(start core.Position, stop core.Position) (protocol.Position, bool) {
	if stop.Line == 0 && stop.Column == 0 {
		startPos, ok := diagnosticStartPosition(start)
		if !ok {
			return protocol.Position{}, false
		}
		return nextCharacterPosition(startPos), true
	}
	if stop.Line <= 0 {
		return protocol.Position{}, false
	}

	return protocol.Position{
		Line:      clampToUint32(stop.Line - 1),
		Character: clampToUint32(stop.Column),
	}, true
}

func nextCharacterPosition(pos protocol.Position) protocol.Position {
	if pos.Character == math.MaxUint32 {
		return pos
	}
	return protocol.Position{
		Line:      pos.Line,
		Character: pos.Character + 1,
	}
}

func ptr[T any](v T) *T {
	return &v
}
