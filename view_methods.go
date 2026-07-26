package main

import (
	"errors"
	"strings"

	"github.com/nevalang/neva-lsp/internal/viewservice"
	src "github.com/nevalang/neva/pkg/ast"
	"github.com/nevalang/neva/pkg/view"
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
	if params.FileID == "" {
		return nil, errors.New("fileId is required")
	}

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

func findEntityInFile(file view.File, targetEntityID string) (ResolveEntityRefResult, bool) {
	for _, component := range file.Components {
		if component.ID == targetEntityID {
			return ResolveEntityRefResult{
				TargetKind:     "component_entity",
				TargetName:     component.Name,
				TargetFileID:   file.ID,
				TargetEntityID: component.ID,
				TargetAnchor:   component.Anchor,
			}, true
		}
	}

	for _, iface := range file.Interfaces {
		if iface.ID == targetEntityID {
			return ResolveEntityRefResult{
				TargetKind:     "interface_entity",
				TargetName:     iface.Name,
				TargetFileID:   file.ID,
				TargetEntityID: iface.ID,
				TargetAnchor:   iface.Anchor,
			}, true
		}
	}

	for _, typ := range file.Types {
		if typ.ID == targetEntityID {
			return ResolveEntityRefResult{
				TargetKind:     "type_entity",
				TargetName:     typ.Name,
				TargetFileID:   file.ID,
				TargetEntityID: typ.ID,
				TargetAnchor:   typ.Anchor,
			}, true
		}
	}

	for _, cnst := range file.Consts {
		if cnst.ID == targetEntityID {
			return ResolveEntityRefResult{
				TargetKind:     "const_entity",
				TargetName:     cnst.Name,
				TargetFileID:   file.ID,
				TargetEntityID: cnst.ID,
				TargetAnchor:   cnst.Anchor,
			}, true
		}
	}

	return ResolveEntityRefResult{}, false
}

func appendEntityMatches(
	results *[]SearchEntitiesResultItem,
	file view.File,
	modulePath string,
	packageName string,
	query string,
	allowedKinds map[string]struct{},
	limit int,
) {
	appendMatch := func(kind string, name string, id string, anchor view.SourceAnchor) {
		if len(*results) >= limit || !containsFold(name, query) {
			return
		}
		if !kindAllowed(allowedKinds, kind) {
			return
		}
		*results = append(*results, SearchEntitiesResultItem{
			Label:    name,
			Kind:     kind,
			Module:   modulePath,
			Package:  packageName,
			FileID:   file.ID,
			EntityID: id,
			Anchor:   anchor,
		})
	}

	for _, entity := range file.Components {
		appendMatch("component", entity.Name, entity.ID, entity.Anchor)
	}
	for _, entity := range file.Interfaces {
		appendMatch("interface", entity.Name, entity.ID, entity.Anchor)
	}
	for _, entity := range file.Types {
		appendMatch("type", entity.Name, entity.ID, entity.Anchor)
	}
	for _, entity := range file.Consts {
		appendMatch("const", entity.Name, entity.ID, entity.Anchor)
	}
}

func kindAllowed(allowedKinds map[string]struct{}, kind string) bool {
	if len(allowedKinds) == 0 {
		return true
	}
	_, ok := allowedKinds[kind]
	return ok
}

func containsFold(s string, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func normalizeFilters(filters []string, legacy string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, item := range filters {
		item = strings.TrimSpace(item)
		if item != "" {
			result[item] = struct{}{}
		}
	}
	legacy = strings.TrimSpace(legacy)
	if legacy != "" {
		result[legacy] = struct{}{}
	}
	return result
}

func isInSet(set map[string]struct{}, value string) bool {
	_, ok := set[value]
	return ok
}

func filterProgramModules(program view.Program, params GetProgramViewRequest) view.Program {
	includeCurrent := boolDefault(params.IncludeCurrent, true)
	includeDeps := boolDefault(params.IncludeDeps, true)
	includeStd := boolDefault(params.IncludeStd, true)

	if includeCurrent && includeDeps && includeStd {
		return program
	}

	filtered := view.Program{Modules: make([]view.Module, 0, len(program.Modules))}
	for _, module := range program.Modules {
		switch classifyModule(module.Path) {
		case "current":
			if includeCurrent {
				filtered.Modules = append(filtered.Modules, module)
			}
		case "std":
			if includeStd {
				filtered.Modules = append(filtered.Modules, module)
			}
		default:
			if includeDeps {
				filtered.Modules = append(filtered.Modules, module)
			}
		}
	}

	return filtered
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func classifyModule(path string) string {
	switch path {
	case "@":
		return "current"
	case "std":
		return "std"
	default:
		return "deps"
	}
}
