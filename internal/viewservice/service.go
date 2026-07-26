// Package viewservice implements the transport-neutral visual-editor queries.
package viewservice

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/nevalang/neva/pkg/ast"
	"github.com/nevalang/neva/pkg/core"
	"github.com/nevalang/neva/pkg/view"
)

func Program(build ast.Build, params ProgramRequest) view.Program {
	return FilterProgramModules(view.ProjectProgram(build), params)
}

func File(build ast.Build, params FileRequest) (view.File, error) {
	if params.FileID == "" {
		return view.File{}, errors.New("fileId is required")
	}
	fileView, found := projectFile(build, params.FileID)
	if !found {
		return view.File{}, fmt.Errorf("file not found: %s", params.FileID)
	}
	return fileView, nil
}

func Resolve(build ast.Build, params ResolveRequest) (ResolveResult, error) {
	if params.TargetFileID == "" {
		return ResolveResult{}, errors.New("targetFileId is required")
	}
	if params.TargetEntityID == "" {
		return ResolveResult{}, errors.New("targetEntityId is required")
	}
	fileView, found := view.ProjectFileByID(build, params.TargetFileID)
	if !found {
		return ResolveResult{}, fmt.Errorf("file not found: %s", params.TargetFileID)
	}
	result, found := findEntityInFile(fileView, params.TargetEntityID)
	if !found {
		return ResolveResult{}, fmt.Errorf("entity not found: %s", params.TargetEntityID)
	}
	return result, nil
}

func Search(build ast.Build, params SearchRequest) ([]SearchResultItem, error) {
	query := strings.TrimSpace(strings.ToLower(params.Query))
	if query == "" {
		return []SearchResultItem{}, nil
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}
	allowedKinds := map[string]struct{}{}
	for _, kind := range params.Kinds {
		if kind = strings.ToLower(strings.TrimSpace(kind)); kind != "" {
			allowedKinds[kind] = struct{}{}
		}
	}
	moduleFilters := normalizeFilters(params.ModuleFilters, params.ModuleFilter)
	packageFilters := normalizeFilters(params.PackageFilters, params.PackageFilter)
	program := view.ProjectProgram(build)
	results := make([]SearchResultItem, 0, limit)
	for _, module := range program.Modules {
		if len(moduleFilters) > 0 && !isInSet(moduleFilters, module.Path) {
			continue
		}
		for _, pkg := range module.Packages {
			qualifiedPackage := module.Path + "/" + pkg.Name
			if len(packageFilters) > 0 && !isInSet(packageFilters, qualifiedPackage) {
				continue
			}
			for _, summary := range pkg.FileSummaries {
				if len(results) >= limit {
					return results, nil
				}
				fileView, found := view.ProjectFileByID(build, summary.ID)
				if !found {
					continue
				}
				appendEntityMatches(&results, fileView, module.Path, pkg.Name, query, allowedKinds, limit)
			}
		}
	}
	return results, nil
}

func FilterProgramModules(program view.Program, params ProgramRequest) view.Program {
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

func projectFile(build ast.Build, fileID string) (view.File, bool) {
	fileView, found := view.ProjectFileByID(build, fileID)
	if !found {
		return view.File{}, false
	}
	restoreDeclaredConstTypes(&fileView)
	sortProjectedFileEntities(&fileView)
	addChainTriggerConnections(&fileView, build)
	return fileView, true
}

func findEntityInFile(file view.File, target string) (ResolveResult, bool) {
	for _, entity := range file.Components {
		if entity.ID == target {
			return ResolveResult{"component_entity", entity.Name, file.ID, entity.ID, entity.Anchor}, true
		}
	}
	for _, entity := range file.Interfaces {
		if entity.ID == target {
			return ResolveResult{"interface_entity", entity.Name, file.ID, entity.ID, entity.Anchor}, true
		}
	}
	for _, entity := range file.Types {
		if entity.ID == target {
			return ResolveResult{"type_entity", entity.Name, file.ID, entity.ID, entity.Anchor}, true
		}
	}
	for _, entity := range file.Consts {
		if entity.ID == target {
			return ResolveResult{"const_entity", entity.Name, file.ID, entity.ID, entity.Anchor}, true
		}
	}
	return ResolveResult{}, false
}

func appendEntityMatches(results *[]SearchResultItem, file view.File, module, pkg, query string, kinds map[string]struct{}, limit int) {
	appendMatch := func(kind, name, id string, anchor view.SourceAnchor) {
		if len(*results) < limit && containsFold(name, query) && kindAllowed(kinds, kind) {
			*results = append(*results, SearchResultItem{Label: name, Kind: kind, Module: module, Package: pkg, FileID: file.ID, EntityID: id, Anchor: anchor})
		}
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

func restoreDeclaredConstTypes(fileView *view.File) {
	for idx := range fileView.Consts {
		if declared := declaredConstTypeFromAnchor(fileView.Consts[idx].Name, fileView.Consts[idx].Anchor.Text); declared != "" {
			fileView.Consts[idx].Type = declared
		}
	}
}
func declaredConstTypeFromAnchor(name, text string) string {
	compact := strings.TrimSpace(text)
	if compact == "" || !strings.HasPrefix(compact, name) {
		return ""
	}
	rest := strings.TrimPrefix(compact, name)
	index := strings.Index(rest, "=")
	if index < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:index])
}
func sortProjectedFileEntities(fileView *view.File) {
	sort.SliceStable(fileView.Components, func(i, j int) bool {
		return entitySortKey(fileView.Components[i].Name, fileView.Components[i].ID) < entitySortKey(fileView.Components[j].Name, fileView.Components[j].ID)
	})
	sort.SliceStable(fileView.Interfaces, func(i, j int) bool {
		return entitySortKey(fileView.Interfaces[i].Name, fileView.Interfaces[i].ID) < entitySortKey(fileView.Interfaces[j].Name, fileView.Interfaces[j].ID)
	})
	sort.SliceStable(fileView.Types, func(i, j int) bool {
		return entitySortKey(fileView.Types[i].Name, fileView.Types[i].ID) < entitySortKey(fileView.Types[j].Name, fileView.Types[j].ID)
	})
	sort.SliceStable(fileView.Consts, func(i, j int) bool {
		return entitySortKey(fileView.Consts[i].Name, fileView.Consts[i].ID) < entitySortKey(fileView.Consts[j].Name, fileView.Consts[j].ID)
	})
}
func entitySortKey(name, id string) string {
	return strings.ToLower(strings.TrimSpace(name)) + "\x00" + id
}
func addChainTriggerConnections(fileView *view.File, build ast.Build) {
	astFile, found := astFileForView(build, *fileView)
	if !found {
		return
	}
	for idx := range fileView.Components {
		componentView := &fileView.Components[idx]
		entity, found := astFile.Entities[componentView.Name]
		if !found || entity.Kind != ast.ComponentEntity || componentView.OverloadIndex >= len(entity.Component) {
			continue
		}
		addComponentChainTriggerConnections(componentView, entity.Component[componentView.OverloadIndex])
	}
}
func astFileForView(build ast.Build, fileView view.File) (ast.File, bool) {
	for modRef, mod := range build.Modules {
		if modRef.Path != fileView.Location.ModulePath {
			continue
		}
		pkg, found := mod.Packages[fileView.Location.Package]
		if !found {
			continue
		}
		file, found := pkg[fileView.Location.File]
		return file, found
	}
	return ast.File{}, false
}
func addComponentChainTriggerConnections(componentView *view.Component, component ast.Component) {
	existing := map[string]struct{}{}
	for _, connection := range componentView.Connections {
		existing[connectionEndpointKey(connection.Sender)+"->"+connectionEndpointKey(connection.Receiver)] = struct{}{}
	}
	ordinal := len(componentView.Connections)
	for _, connection := range component.Net {
		ordinal = addChainTriggers(componentView, existing, ordinal, connection.Senders, connection.Receivers, connection.Meta, 0)
	}
}
func addChainTriggers(componentView *view.Component, existing map[string]struct{}, ordinal int, outer []ast.ConnectionSender, receivers []ast.ConnectionReceiver, meta core.Meta, depth int) int {
	for _, receiver := range receivers {
		if receiver.ChainedConnection == nil {
			continue
		}
		chained := receiver.ChainedConnection
		next := make([]ast.ConnectionSender, 0, len(outer)*len(chained.Senders))
		for _, outerSender := range outer {
			source := endpointFromConnectionSender(outerSender)
			for _, head := range chained.Senders {
				next = append(next, mergeChainSender(outerSender, head))
				if !isConcreteConnectionSender(head) {
					continue
				}
				target := endpointFromConnectionSender(head)
				key := connectionEndpointKey(source) + "->" + connectionEndpointKey(target)
				if _, found := existing[key]; found {
					continue
				}
				existing[key] = struct{}{}
				componentView.Connections = append(componentView.Connections, view.Connection{ID: componentView.ID + "/connection/chain-trigger/" + strconv.Itoa(ordinal), Sender: source, Receiver: target, Anchor: sourceAnchorFromMeta(meta), ChainDepth: depth, ChainPath: []string{"chain:trigger"}, Signature: key})
				ordinal++
			}
		}
		ordinal = addChainTriggers(componentView, existing, ordinal, next, chained.Receivers, chained.Meta, depth+1)
	}
	return ordinal
}
func isConcreteConnectionSender(sender ast.ConnectionSender) bool {
	return sender.PortAddr != nil || sender.Const != nil
}
func mergeChainSender(outer, chained ast.ConnectionSender) ast.ConnectionSender {
	merged := chained
	if !isConcreteConnectionSender(merged) {
		merged = outer
	}
	merged.StructSelector = append(append([]string{}, outer.StructSelector...), chained.StructSelector...)
	return merged
}
func endpointFromConnectionSender(sender ast.ConnectionSender) view.ConnectionEndpoint {
	if sender.Const != nil {
		return view.ConnectionEndpoint{Kind: "const", ConstType: sender.Const.TypeExpr.String(), ConstValue: sender.Const.Value.String(), Selector: append([]string{}, sender.StructSelector...), Anchor: sourceAnchorFromMeta(sender.Const.Meta)}
	}
	endpoint := endpointFromPortAddr(sender.PortAddr)
	endpoint.Selector = append([]string{}, sender.StructSelector...)
	endpoint.Anchor = sourceAnchorFromMeta(sender.Meta)
	return endpoint
}
func endpointFromPortAddr(addr *ast.PortAddr) view.ConnectionEndpoint {
	if addr == nil {
		return view.ConnectionEndpoint{Kind: "port"}
	}
	return view.ConnectionEndpoint{Kind: "port", Node: addr.Node, Port: addr.Port, Index: addr.Idx, Selector: []string{}, Anchor: sourceAnchorFromMeta(addr.Meta)}
}
func connectionEndpointKey(endpoint view.ConnectionEndpoint) string {
	if endpoint.Kind == "const" {
		return "const:" + endpoint.ConstType + "=" + endpoint.ConstValue + "." + strings.Join(endpoint.Selector, ".")
	}
	index := ""
	if endpoint.Index != nil {
		index = fmt.Sprintf("[%d]", *endpoint.Index)
	}
	return "port:" + endpoint.Node + ":" + endpoint.Port + index + "." + strings.Join(endpoint.Selector, ".")
}
func sourceAnchorFromMeta(meta core.Meta) view.SourceAnchor {
	return view.SourceAnchor{ModulePath: meta.Location.ModRef.Path, Package: meta.Location.Package, File: meta.Location.Filename, Text: meta.Text, StartLine: meta.Start.Line, StartCol: meta.Start.Column, EndLine: meta.Stop.Line, EndCol: meta.Stop.Column}
}
func kindAllowed(kinds map[string]struct{}, kind string) bool {
	if len(kinds) == 0 {
		return true
	}
	_, ok := kinds[kind]
	return ok
}
func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
func normalizeFilters(filters []string, legacy string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, item := range filters {
		if item = strings.TrimSpace(item); item != "" {
			result[item] = struct{}{}
		}
	}
	if legacy = strings.TrimSpace(legacy); legacy != "" {
		result[legacy] = struct{}{}
	}
	return result
}
func isInSet(set map[string]struct{}, value string) bool { _, ok := set[value]; return ok }
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
