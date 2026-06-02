package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/nevalang/neva/pkg/ast"
	"github.com/nevalang/neva/pkg/core"
	"github.com/nevalang/neva/pkg/view"
)

func projectFileView(build ast.Build, fileID string) (view.File, bool) {
	fileView, found := view.ProjectFileByID(build, fileID)
	if !found {
		return view.File{}, false
	}
	sortProjectedFileEntities(&fileView)
	addChainTriggerConnections(&fileView, build)
	return fileView, true
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

func entitySortKey(name string, id string) string {
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

	nextOrdinal := len(componentView.Connections)
	for _, connection := range component.Net {
		nextOrdinal = addChainTriggersFromReceivers(componentView, existing, nextOrdinal, connection.Senders, connection.Receivers, connection.Meta, 0)
	}
}

func addChainTriggersFromReceivers(
	componentView *view.Component,
	existing map[string]struct{},
	nextOrdinal int,
	outerSenders []ast.ConnectionSender,
	receivers []ast.ConnectionReceiver,
	anchorMeta core.Meta,
	depth int,
) int {
	for _, receiver := range receivers {
		if receiver.ChainedConnection == nil {
			continue
		}
		chained := receiver.ChainedConnection
		nextOuterSenders := make([]ast.ConnectionSender, 0, len(outerSenders)*len(chained.Senders))
		for _, outerSender := range outerSenders {
			source := endpointFromConnectionSender(outerSender)
			for _, chainHead := range chained.Senders {
				nextOuterSenders = append(nextOuterSenders, mergeChainSender(outerSender, chainHead))
				if !isConcreteConnectionSender(chainHead) {
					continue
				}
				target := endpointFromConnectionSender(chainHead)
				key := connectionEndpointKey(source) + "->" + connectionEndpointKey(target)
				if _, found := existing[key]; found {
					continue
				}
				existing[key] = struct{}{}
				componentView.Connections = append(componentView.Connections, view.Connection{
					ID:               componentView.ID + "/connection/chain-trigger/" + strconv.Itoa(nextOrdinal),
					Sender:           source,
					Receiver:         target,
					Anchor:           sourceAnchorFromMeta(anchorMeta),
					ChainDepth:       depth,
					ChainPath:        []string{"chain:trigger"},
					Signature:        key,
					DuplicateOrdinal: 0,
				})
				nextOrdinal++
			}
		}
		nextOrdinal = addChainTriggersFromReceivers(componentView, existing, nextOrdinal, nextOuterSenders, chained.Receivers, chained.Meta, depth+1)
	}
	return nextOrdinal
}

func isConcreteConnectionSender(sender ast.ConnectionSender) bool {
	return sender.PortAddr != nil || sender.Const != nil
}

func mergeChainSender(outer ast.ConnectionSender, chained ast.ConnectionSender) ast.ConnectionSender {
	merged := chained
	if !isConcreteConnectionSender(merged) {
		merged = outer
	}
	merged.StructSelector = append(append([]string{}, outer.StructSelector...), chained.StructSelector...)
	return merged
}

func endpointFromConnectionSender(sender ast.ConnectionSender) view.ConnectionEndpoint {
	if sender.Const != nil {
		return view.ConnectionEndpoint{
			Kind:       "const",
			ConstType:  sender.Const.TypeExpr.String(),
			ConstValue: sender.Const.Value.String(),
			Selector:   append([]string{}, sender.StructSelector...),
			Anchor:     sourceAnchorFromMeta(sender.Const.Meta),
		}
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
	return view.ConnectionEndpoint{
		Kind:     "port",
		Node:     addr.Node,
		Port:     addr.Port,
		Index:    addr.Idx,
		Selector: []string{},
		Anchor:   sourceAnchorFromMeta(addr.Meta),
	}
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
	return view.SourceAnchor{
		ModulePath: meta.Location.ModRef.Path,
		Package:    meta.Location.Package,
		File:       meta.Location.Filename,
		Text:       meta.Text,
		StartLine:  meta.Start.Line,
		StartCol:   meta.Start.Column,
		EndLine:    meta.Stop.Line,
		EndCol:     meta.Stop.Column,
	}
}
