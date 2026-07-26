// CodeLens handlers add actionable inline annotations above declarations.
// In Neva LSP we currently expose entity-level lenses for references and interface implementations,
// then resolve them into VS Code's show-references command payload.
package main

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	src "github.com/nevalang/neva/pkg/ast"
	"github.com/nevalang/neva/pkg/core"
	ts "github.com/nevalang/neva/pkg/typesystem"
)

type codeLensData struct {
	// URI points to the source file where the lens was created.
	URI string `json:"uri"`
	// Name is the entity name lens resolution should target.
	Name string `json:"name"`
	// Kind selects which query to run for the target entity.
	Kind codeLensKind `json:"kind"`
}

type codeLensKind string

const (
	codeLensKindReferences      codeLensKind = "references"
	codeLensKindImplementations codeLensKind = "implementations"
)

const showReferencesClientCommand = "neva.showReferences"

// TextDocumentCodeLens emits per-entity code lenses for references and, for interfaces, implementations.
func (s *Server) TextDocumentCodeLens(
	glspCtx *glsp.Context,
	params *protocol.CodeLensParams,
) ([]protocol.CodeLens, error) {
	build, ok := s.getBuild()
	if !ok {
		return []protocol.CodeLens{}, nil
	}

	fileCtx, err := s.findFile(build, params.TextDocument.URI)
	if err != nil {
		if isFileNotFoundInBuild(err) {
			s.logReadOnlyFileMiss("textDocument/codeLens", params.TextDocument.URI, err)
			return []protocol.CodeLens{}, nil
		}
		return nil, err
	}

	lenses := make([]protocol.CodeLens, 0, len(fileCtx.file.Entities)*2)
	missingMetaCount := 0
	for name, entity := range fileCtx.file.Entities {
		nameRange, ok := rangeForEntityDeclaration(entity, name, fileCtx.filePath)
		if !ok {
			missingMetaCount++
			continue
		}

		target, resolved := s.resolveEntityRef(build, fileCtx, core.EntityRef{Name: name})
		if !resolved {
			continue
		}

		if len(s.referenceLocationsForEntity(build, target)) > 0 {
			lenses = append(lenses, protocol.CodeLens{
				Range: nameRange,
				Data: codeLensData{
					URI:  pathToURI(fileCtx.filePath),
					Name: name,
					Kind: codeLensKindReferences,
				},
			})
		}

		if entity.Kind == src.InterfaceEntity {
			if len(s.implementationLocationsForEntity(build, target)) == 0 {
				continue
			}
			lenses = append(lenses, protocol.CodeLens{
				Range: nameRange,
				Data: codeLensData{
					URI:  pathToURI(fileCtx.filePath),
					Name: name,
					Kind: codeLensKindImplementations,
				},
			})
		}
	}
	if missingMetaCount > 0 {
		s.logger.Info("skipped code lenses for entities without metadata", "count", missingMetaCount, "file", fileCtx.filePath)
	}

	return lenses, nil
}

// CodeLensResolve computes locations and attaches the show-references command payload.
func (s *Server) CodeLensResolve(
	glspCtx *glsp.Context,
	lens *protocol.CodeLens,
) (*protocol.CodeLens, error) {
	parsedCodeLensData, ok := parseCodeLensData(lens.Data)
	if !ok {
		return lens, nil
	}

	build, ok := s.getBuild()
	if !ok {
		return lens, nil
	}

	fileCtx, err := s.findFile(build, parsedCodeLensData.URI)
	if err != nil {
		if isFileNotFoundInBuild(err) {
			s.logReadOnlyFileMiss("codeLens/resolve", parsedCodeLensData.URI, err)
			return lens, nil
		}
		return nil, err
	}

	target, ok := s.resolveEntityRef(build, fileCtx, core.EntityRef{Name: parsedCodeLensData.Name})
	if !ok {
		return lens, nil
	}

	switch parsedCodeLensData.Kind {
	case codeLensKindImplementations:
		locations := s.implementationLocationsForEntity(build, target)
		if len(locations) == 0 {
			lens.Command = nil
			return lens, nil
		}
		title := fmt.Sprintf("%d implementations", len(locations))
		lens.Command = buildShowReferencesCommand(parsedCodeLensData.URI, lens.Range.Start, locations, title)
	case codeLensKindReferences:
		locations := s.referenceLocationsForEntity(build, target)
		if len(locations) == 0 {
			lens.Command = nil
			return lens, nil
		}
		title := fmt.Sprintf("%d references", len(locations))
		lens.Command = buildShowReferencesCommand(parsedCodeLensData.URI, lens.Range.Start, locations, title)
	default:
		s.logger.Info("skipped code lens resolve for unknown kind", "kind", parsedCodeLensData.Kind, "name", parsedCodeLensData.Name)
		return lens, nil
	}

	return lens, nil
}

// parseCodeLensData decodes strongly-typed lens metadata from LSP's generic data field.
func parseCodeLensData(raw any) (codeLensData, bool) {
	parsedCodeLensData := codeLensData{}
	rawJSON, err := json.Marshal(raw)
	if err != nil {
		return parsedCodeLensData, false
	}
	if err := json.Unmarshal(rawJSON, &parsedCodeLensData); err != nil {
		return parsedCodeLensData, false
	}
	if parsedCodeLensData.URI == "" || parsedCodeLensData.Name == "" {
		return parsedCodeLensData, false
	}
	return parsedCodeLensData, isKnownCodeLensKind(parsedCodeLensData.Kind)
}

func isKnownCodeLensKind(kind codeLensKind) bool {
	return kind == codeLensKindReferences || kind == codeLensKindImplementations
}

// referenceLocationsForEntity includes only explicit textual references.
func (s *Server) referenceLocationsForEntity(build *src.Build, target *resolvedEntity) []protocol.Location {
	return s.referencesForEntity(build, target)
}

// implementationLocationsForEntity returns implementation-related locations for interfaces.
func (s *Server) implementationLocationsForEntity(build *src.Build, target *resolvedEntity) []protocol.Location {
	if target.entity.Kind != src.InterfaceEntity {
		return []protocol.Location{}
	}
	return s.implementationLocationsForInterface(build, target)
}

// implementationLocationsForInterface finds all components that structurally implement the interface.
func (s *Server) implementationLocationsForInterface(build *src.Build, ifaceTarget *resolvedEntity) []protocol.Location {
	interfaceDef := ifaceTarget.entity.Interface
	locations := []protocol.Location{}
	seen := map[protocol.Location]struct{}{}
	s.forEachWorkspaceFile(build, func(fileCtx fileContext) bool {
		for componentName, entity := range fileCtx.file.Entities {
			if entity.Kind != src.ComponentEntity {
				continue
			}
			for _, component := range entity.Component {
				if !componentImplementsInterface(component.IO, interfaceDef) {
					continue
				}
				componentMeta := entity.Meta()
				if componentMeta == nil {
					continue
				}
				componentRange, ok := rangeForEntityDeclaration(entity, componentName, fileCtx.filePath)
				if !ok {
					componentRange = rangeForName(*componentMeta, componentName, 0)
				}
				location := protocol.Location{
					URI:   pathToURI(fileCtx.filePath),
					Range: componentRange,
				}
				if _, ok := seen[location]; ok {
					continue
				}
				seen[location] = struct{}{}
				locations = append(locations, location)
			}
		}
		return true
	})
	return locations
}

// implementedInterfacesForComponent returns interfaces that are structurally implemented by a component.
func (s *Server) implementedInterfacesForComponent(build *src.Build, componentTarget *resolvedEntity) []*resolvedEntity {
	if componentTarget.entity.Kind != src.ComponentEntity || len(componentTarget.entity.Component) == 0 {
		return nil
	}
	componentIO := componentTarget.entity.Component[0].IO
	implementedInterfaces := []*resolvedEntity{}
	s.forEachWorkspaceFile(build, func(fileCtx fileContext) bool {
		for interfaceName, entity := range fileCtx.file.Entities {
			if entity.Kind != src.InterfaceEntity {
				continue
			}
			if !componentImplementsInterface(componentIO, entity.Interface) {
				continue
			}
			implementedInterfaces = append(implementedInterfaces, &resolvedEntity{
				moduleRef:   fileCtx.moduleRef,
				packageName: fileCtx.packageName,
				name:        interfaceName,
				filePath:    fileCtx.filePath,
				entity:      entity,
			})
		}
		return true
	})
	return implementedInterfaces
}

// componentImplementsInterface performs a structural port-level check for MVP interface implementation.
func componentImplementsInterface(componentIO src.IO, interfaceDef src.Interface) bool {
	interfaceTypeParams := interfaceTypeParamNames(interfaceDef.TypeParams)
	return ioContainsPorts(componentIO.In, interfaceDef.IO.In, interfaceTypeParams) &&
		ioContainsPorts(componentIO.Out, interfaceDef.IO.Out, interfaceTypeParams)
}

func interfaceTypeParamNames(typeParams src.TypeParams) map[string]struct{} {
	result := make(map[string]struct{}, len(typeParams.Params))
	for _, param := range typeParams.Params {
		if param.Name == "" {
			continue
		}
		result[param.Name] = struct{}{}
	}
	return result
}

func ioContainsPorts(
	componentPorts map[string]src.Port,
	interfacePorts map[string]src.Port,
	interfaceTypeParams map[string]struct{},
) bool {
	if len(interfacePorts) == 1 && len(componentPorts) == 1 {
		componentPort := firstPort(componentPorts)
		interfacePort := firstPort(interfacePorts)
		if componentPort.IsArray != interfacePort.IsArray {
			return false
		}
		return typeExprMatchesWithInterfaceTypeParams(componentPort.TypeExpr, interfacePort.TypeExpr, interfaceTypeParams)
	}

	interfacePortNames := make([]string, 0, len(interfacePorts))
	for name := range interfacePorts {
		interfacePortNames = append(interfacePortNames, name)
	}
	slices.Sort(interfacePortNames)
	for _, name := range interfacePortNames {
		interfacePort := interfacePorts[name]
		componentPort, ok := componentPorts[name]
		if !ok {
			return false
		}
		if componentPort.IsArray != interfacePort.IsArray {
			return false
		}
		if !typeExprMatchesWithInterfaceTypeParams(componentPort.TypeExpr, interfacePort.TypeExpr, interfaceTypeParams) {
			return false
		}
	}
	return true
}

func firstPort(ports map[string]src.Port) src.Port {
	for _, port := range ports {
		return port
	}
	return src.Port{}
}

func typeExprMatchesWithInterfaceTypeParams(
	componentExpr ts.Expr,
	interfaceExpr ts.Expr,
	interfaceTypeParams map[string]struct{},
) bool {
	if interfaceExpr.Inst != nil &&
		interfaceExpr.Inst.Ref.Pkg == "" &&
		len(interfaceExpr.Inst.Args) == 0 {
		if _, ok := interfaceTypeParams[interfaceExpr.Inst.Ref.Name]; ok {
			return true
		}
	}

	if componentExpr.Inst != nil || interfaceExpr.Inst != nil {
		if componentExpr.Inst == nil || interfaceExpr.Inst == nil {
			return false
		}
		if !sameTypeRef(componentExpr.Inst.Ref, interfaceExpr.Inst.Ref) {
			return false
		}
		if len(componentExpr.Inst.Args) != len(interfaceExpr.Inst.Args) {
			return false
		}
		for i := range interfaceExpr.Inst.Args {
			if !typeExprMatchesWithInterfaceTypeParams(
				componentExpr.Inst.Args[i],
				interfaceExpr.Inst.Args[i],
				interfaceTypeParams,
			) {
				return false
			}
		}
		return true
	}

	if componentExpr.Lit == nil || interfaceExpr.Lit == nil {
		return false
	}
	if componentExpr.Lit.Type() != interfaceExpr.Lit.Type() {
		return false
	}

	switch interfaceExpr.Lit.Type() {
	case ts.StructLitType:
		if len(componentExpr.Lit.Struct) != len(interfaceExpr.Lit.Struct) {
			return false
		}
		for fieldName, interfaceFieldExpr := range interfaceExpr.Lit.Struct {
			componentFieldExpr, ok := componentExpr.Lit.Struct[fieldName]
			if !ok {
				return false
			}
			if !typeExprMatchesWithInterfaceTypeParams(componentFieldExpr, interfaceFieldExpr, interfaceTypeParams) {
				return false
			}
		}
		return true
	case ts.UnionLitType:
		if len(componentExpr.Lit.Union) != len(interfaceExpr.Lit.Union) {
			return false
		}
		for tagName, interfaceTagExpr := range interfaceExpr.Lit.Union {
			componentTagExpr, ok := componentExpr.Lit.Union[tagName]
			if !ok {
				return false
			}
			if interfaceTagExpr == nil || componentTagExpr == nil {
				if interfaceTagExpr != nil || componentTagExpr != nil {
					return false
				}
				continue
			}
			if !typeExprMatchesWithInterfaceTypeParams(*componentTagExpr, *interfaceTagExpr, interfaceTypeParams) {
				return false
			}
		}
		return true
	case ts.EmptyLitType:
		return true
	default:
		return false
	}
}

func sameTypeRef(a core.EntityRef, b core.EntityRef) bool {
	return a.Pkg == b.Pkg && a.Name == b.Name
}

// buildShowReferencesCommand creates the editor command expected by VS Code's references UI.
func buildShowReferencesCommand(
	uri string,
	position protocol.Position,
	locations []protocol.Location,
	title string,
) *protocol.Command {
	return &protocol.Command{
		Title:   title,
		Command: showReferencesClientCommand,
		Arguments: []any{
			uri,
			position,
			locations,
		},
	}
}
