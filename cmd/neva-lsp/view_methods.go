package main

import (
	"errors"

	"github.com/nevalang/neva-tools/internal/viewservice"
	src "github.com/nevalang/neva/pkg/ast"
	"github.com/tliron/glsp"
)

func (s *Server) GetProgramView(_ *glsp.Context, params GetProgramViewRequest) (any, error) {
	build, ok := s.getBuild()
	if !ok {
		return nil, errors.New("program index is not ready")
	}
	return viewservice.Program(*build, params), nil
}

func (s *Server) GetFileView(_ *glsp.Context, params GetFileViewRequest) (any, error) {
	build, ok := s.getBuild()
	if !ok {
		return nil, errors.New("program index is not ready")
	}
	return viewservice.File(*build, params)
}

func (s *Server) ResolveEntityRef(_ *glsp.Context, params ResolveEntityRefRequest) (any, error) {
	build, ok := s.getBuild()
	if !ok {
		return nil, errors.New("program index is not ready")
	}
	return viewservice.Resolve(*build, params)
}

func (s *Server) SearchEntities(_ *glsp.Context, params SearchEntitiesRequest) (any, error) {
	build, ok := s.getBuild()
	if !ok {
		return nil, errors.New("program index is not ready")
	}
	return viewservice.Search(*build, params)
}

// ResolveFileLegacy is retained only as migration reference.
// It is intentionally NOT registered in LSP method dispatch anymore.
//
// Deprecated: use neva/view/getFileView.
func (s *Server) ResolveFileLegacy(_ *glsp.Context, params LegacyGetFileViewRequest) (any, error) {
	s.logger.Info("resolve_file is deprecated; use neva/view/getFileView")

	build, ok := s.getBuild()
	if !ok {
		return nil, errors.New("program index is not ready")
	}

	uriPath := params.Document.URI.Path
	if uriPath == "" {
		uriPath = params.Document.URI.FSPath
	}
	ctx, err := s.findFile(build, uriPath)
	if err != nil {
		return nil, err
	}

	return LegacyGetFileViewResponse{
		File:  ctx.file,
		Extra: Extra{NodesPorts: map[string]map[string]src.Interface{}},
	}, nil
}
