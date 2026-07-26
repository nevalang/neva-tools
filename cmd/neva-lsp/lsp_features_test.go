package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	src "github.com/nevalang/neva/pkg/ast"
	"github.com/nevalang/neva/pkg/core"
	"github.com/nevalang/neva/pkg/indexer"
	ts "github.com/nevalang/neva/pkg/typesystem"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// TestGeneralCompletionsIncludesEntitiesAndNodeNames verifies MVP completion coverage:
// package entities and current-component node names must both be included.
func TestGeneralCompletionsIncludesEntitiesAndNodeNames(t *testing.T) {
	t.Parallel()

	moduleRef := core.ModuleRef{Path: "@"}
	build := &src.Build{
		Modules: map[core.ModuleRef]src.Module{
			moduleRef: {
				Packages: map[string]src.Package{
					"main": {
						"file1": {
							Entities: map[string]src.Entity{
								"FooComponent": {Kind: src.ComponentEntity},
								"FooType":      {Kind: src.TypeEntity},
								"FooConst":     {Kind: src.ConstEntity},
								"FooInterface": {Kind: src.InterfaceEntity},
							},
						},
					},
				},
			},
		},
	}

	fileCtx := &fileContext{
		moduleRef:   moduleRef,
		packageName: "main",
		file: src.File{
			Imports: map[string]src.Import{
				"fmt": {Package: "fmt"},
			},
		},
	}
	compCtx := &componentContext{
		component: src.Component{
			Nodes: map[string]src.Node{
				"parser": {},
				"writer": {},
			},
		},
	}

	items := (&Server{}).generalCompletions(build, fileCtx, compCtx)
	itemsByLabel := map[string]protocol.CompletionItem{}
	for _, item := range items {
		itemsByLabel[item.Label] = item
	}

	for _, expectedLabel := range []string{
		"FooComponent",
		"FooType",
		"FooConst",
		"FooInterface",
		"parser",
		"writer",
	} {
		if _, ok := itemsByLabel[expectedLabel]; !ok {
			t.Fatalf("missing completion label %q", expectedLabel)
		}
	}

	assertCompletionKind(t, itemsByLabel, "FooComponent", protocol.CompletionItemKindFunction)
	assertCompletionKind(t, itemsByLabel, "FooType", protocol.CompletionItemKindClass)
	assertCompletionKind(t, itemsByLabel, "FooConst", protocol.CompletionItemKindConstant)
	assertCompletionKind(t, itemsByLabel, "FooInterface", protocol.CompletionItemKindInterface)
	assertCompletionKind(t, itemsByLabel, "parser", protocol.CompletionItemKindVariable)
}

// TestParseCodeLensDataValidation checks strict validation of the generic LSP CodeLens.Data payload.
func TestParseCodeLensDataValidation(t *testing.T) {
	t.Parallel()

	assertParsed := func(testName string, raw any, wantValid bool) {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			_, ok := parseCodeLensData(raw)
			if ok != wantValid {
				t.Fatalf("parseCodeLensData() valid=%v, want %v", ok, wantValid)
			}
		})
	}

	assertParsed("references_kind_is_valid", map[string]any{
		"uri":  "file:///tmp/main.neva",
		"name": "Main",
		"kind": "references",
	}, true)
	assertParsed("implementations_kind_is_valid", map[string]any{
		"uri":  "file:///tmp/main.neva",
		"name": "Main",
		"kind": "implementations",
	}, true)
	assertParsed("unknown_kind_is_rejected", map[string]any{
		"uri":  "file:///tmp/main.neva",
		"name": "Main",
		"kind": "unknown",
	}, false)
	assertParsed("missing_name_is_rejected", map[string]any{
		"uri":  "file:///tmp/main.neva",
		"kind": "references",
	}, false)
}

// TestImplementationLocationsForInterface verifies MVP interface-implementation discovery for CodeLens.
func TestImplementationLocationsForInterface(t *testing.T) {
	t.Parallel()

	ifaceMeta := core.Meta{
		Start: core.Position{Line: 1, Column: 0},
		Stop:  core.Position{Line: 1, Column: 8},
	}
	componentMeta := core.Meta{
		Start: core.Position{Line: 10, Column: 0},
		Stop:  core.Position{Line: 10, Column: 8},
	}

	interfaceEntity := src.Entity{
		Kind:      src.InterfaceEntity,
		Interface: src.Interface{IO: src.IO{In: map[string]src.Port{}, Out: map[string]src.Port{}}, Meta: ifaceMeta},
	}
	componentEntity := src.Entity{
		Kind: src.ComponentEntity,
		Component: []src.Component{
			{
				Interface: src.Interface{IO: src.IO{In: map[string]src.Port{}, Out: map[string]src.Port{}}},
				Meta:      componentMeta,
			},
		},
	}

	moduleRef := core.ModuleRef{Path: "@"}
	build := &src.Build{
		Modules: map[core.ModuleRef]src.Module{
			moduleRef: {
				Packages: map[string]src.Package{
					"main": {
						"iface_file": {
							Entities: map[string]src.Entity{
								"Greeter": interfaceEntity,
							},
						},
						"component_file": {
							Entities: map[string]src.Entity{
								"HelloGreeter": componentEntity,
							},
						},
					},
				},
			},
		},
	}

	server := &Server{workspacePath: "/tmp/workspace"}
	target := &resolvedEntity{
		moduleRef:   moduleRef,
		packageName: "main",
		name:        "Greeter",
		filePath:    "/tmp/workspace/main/iface_file.neva",
		entity:      interfaceEntity,
	}

	locations := server.implementationLocationsForEntity(build, target)
	if len(locations) != 1 {
		t.Fatalf("implementationLocationsForEntity() count=%d, want 1", len(locations))
	}
}

// TestTextDocumentCodeLensEmitsExpectedKinds ensures interface declarations get two lenses
// (references + implementations), while zero-count lenses are omitted.
func TestTextDocumentCodeLensEmitsExpectedKinds(t *testing.T) {
	t.Parallel()

	server, docURI := buildTestLSPServerWithSingleFile()

	lenses, err := server.TextDocumentCodeLens(nil, &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	})
	if err != nil {
		t.Fatalf("TextDocumentCodeLens() error = %v", err)
	}
	if len(lenses) != 2 {
		t.Fatalf("TextDocumentCodeLens() count=%d, want 2", len(lenses))
	}

	// We expect only Greeter references + implementations (other declarations have zero-count lenses).
	got := map[string]map[codeLensKind]struct{}{}
	for _, lens := range lenses {
		parsedCodeLensData, ok := parseCodeLensData(lens.Data)
		if !ok {
			t.Fatalf("invalid code lens data payload: %#v", lens.Data)
		}
		if _, exists := got[parsedCodeLensData.Name]; !exists {
			got[parsedCodeLensData.Name] = map[codeLensKind]struct{}{}
		}
		got[parsedCodeLensData.Name][parsedCodeLensData.Kind] = struct{}{}
	}

	assertLensKinds(t, got, "Greeter", []codeLensKind{codeLensKindReferences, codeLensKindImplementations})
}

// TestCodeLensResolveForInterfaceImplementation verifies resolved implementation lenses
// point to implementing components via show-references command payload.
func TestCodeLensResolveForInterfaceImplementation(t *testing.T) {
	t.Parallel()

	server, docURI := buildTestLSPServerWithSingleFile()
	lens := &protocol.CodeLens{
		Range: protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
		Data: codeLensData{
			URI:  docURI,
			Name: "Greeter",
			Kind: codeLensKindImplementations,
		},
	}

	resolvedLens, err := server.CodeLensResolve(nil, lens)
	if err != nil {
		t.Fatalf("CodeLensResolve() error = %v", err)
	}
	if resolvedLens.Command == nil {
		t.Fatalf("CodeLensResolve() command is nil")
	}
	if resolvedLens.Command.Title != "1 implementations" {
		t.Fatalf("CodeLensResolve() title=%q, want %q", resolvedLens.Command.Title, "1 implementations")
	}
}

// TestCodeLensResolveForInterfaceReferences verifies interface references include only explicit refs.
func TestCodeLensResolveForInterfaceReferences(t *testing.T) {
	t.Parallel()

	server, docURI := buildTestLSPServerWithSingleFile()
	lens := &protocol.CodeLens{
		Range: protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
		Data: codeLensData{
			URI:  docURI,
			Name: "Greeter",
			Kind: codeLensKindReferences,
		},
	}

	resolvedLens, err := server.CodeLensResolve(nil, lens)
	if err != nil {
		t.Fatalf("CodeLensResolve() error = %v", err)
	}
	if resolvedLens.Command == nil {
		t.Fatalf("CodeLensResolve() command is nil")
	}
	if resolvedLens.Command.Title != "1 references" {
		t.Fatalf("CodeLensResolve() title=%q, want %q", resolvedLens.Command.Title, "1 references")
	}
}

func TestCodeLensResolveHidesZeroReferenceLens(t *testing.T) {
	t.Parallel()

	server, docURI := buildTestLSPServerWithSingleFile()
	lens := &protocol.CodeLens{
		Range: protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
		Data: codeLensData{
			URI:  docURI,
			Name: "Answer",
			Kind: codeLensKindReferences,
		},
	}

	resolvedLens, err := server.CodeLensResolve(nil, lens)
	if err != nil {
		t.Fatalf("CodeLensResolve() error = %v", err)
	}
	if resolvedLens.Command != nil {
		t.Fatalf("CodeLensResolve() command=%+v, want nil for zero references", resolvedLens.Command)
	}
}

func TestComponentReferencesDoNotIncludeImplementedInterfaceReferences(t *testing.T) {
	t.Parallel()

	server, docURI := buildTestLSPServerWithSingleFile()
	lens := &protocol.CodeLens{
		Range: protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
		Data: codeLensData{
			URI:  docURI,
			Name: "HelloGreeter",
			Kind: codeLensKindReferences,
		},
	}

	resolvedLens, err := server.CodeLensResolve(nil, lens)
	if err != nil {
		t.Fatalf("CodeLensResolve() error = %v", err)
	}
	if resolvedLens.Command != nil {
		t.Fatalf("CodeLensResolve() command=%+v, want nil for component without direct references", resolvedLens.Command)
	}
}

func TestTextDocumentCodeLensSkipsEmptyImplementationLens(t *testing.T) {
	t.Parallel()

	moduleRef := core.ModuleRef{Path: "@"}
	interfaceMeta := core.Meta{Start: core.Position{Line: 1, Column: 0}, Stop: core.Position{Line: 1, Column: 7}}
	build := src.Build{
		Modules: map[core.ModuleRef]src.Module{
			moduleRef: {
				Packages: map[string]src.Package{
					"main": {
						"main": {
							Entities: map[string]src.Entity{
								"Greeter": {
									Kind: src.InterfaceEntity,
									Interface: src.Interface{
										IO: src.IO{
											In: map[string]src.Port{
												"data": {TypeExpr: ts.Expr{Inst: &ts.InstExpr{Ref: core.EntityRef{Name: "int"}}}},
											},
											Out: map[string]src.Port{
												"res": {TypeExpr: ts.Expr{Inst: &ts.InstExpr{Ref: core.EntityRef{Name: "bool"}}}},
											},
										},
										Meta: interfaceMeta,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	server := &Server{
		workspacePath: "/tmp/workspace",
		indexMutex:    &sync.Mutex{},
	}
	server.setBuild(build)

	docURI := pathToURI("/tmp/workspace/main/main.neva")
	lenses, err := server.TextDocumentCodeLens(nil, &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	})
	if err != nil {
		t.Fatalf("TextDocumentCodeLens() error = %v", err)
	}
	if len(lenses) != 0 {
		t.Fatalf("TextDocumentCodeLens() count=%d, want 0", len(lenses))
	}
}

func TestComponentImplementsInterfaceWithTypeParameterWildcard(t *testing.T) {
	t.Parallel()

	interfaceDef := src.Interface{
		TypeParams: src.TypeParams{
			Params: []ts.Param{
				{
					Name: "T",
					Constr: ts.Expr{
						Inst: &ts.InstExpr{Ref: core.EntityRef{Name: "any"}},
					},
				},
			},
		},
		IO: src.IO{
			In: map[string]src.Port{
				"data": {TypeExpr: ts.Expr{Inst: &ts.InstExpr{Ref: core.EntityRef{Name: "T"}}}},
			},
			Out: map[string]src.Port{
				"res": {TypeExpr: ts.Expr{Inst: &ts.InstExpr{Ref: core.EntityRef{Name: "bool"}}}},
			},
		},
	}

	componentIO := src.IO{
		In: map[string]src.Port{
			"data": {TypeExpr: ts.Expr{Inst: &ts.InstExpr{Ref: core.EntityRef{Name: "int"}}}},
		},
		Out: map[string]src.Port{
			"res": {TypeExpr: ts.Expr{Inst: &ts.InstExpr{Ref: core.EntityRef{Name: "bool"}}}},
		},
	}

	if !componentImplementsInterface(componentIO, interfaceDef) {
		t.Fatal("componentImplementsInterface() = false, want true for type-parameter wildcard match")
	}
}

func TestRangeForEntityDeclarationFallbackWhenMetaMissing(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "types.neva")
	content := "pub type any\npub type int\n"
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	entity := src.Entity{
		Kind: src.TypeEntity,
		Type: ts.Def{},
	}

	rng, ok := rangeForEntityDeclaration(entity, "int", filePath)
	if !ok {
		t.Fatal("rangeForEntityDeclaration() ok=false, want true")
	}
	if rng.Start.Line != 1 || rng.Start.Character != 9 {
		t.Fatalf(
			"rangeForEntityDeclaration() start=%d:%d, want 1:9",
			rng.Start.Line,
			rng.Start.Character,
		)
	}
	if rng.End.Line != 1 || rng.End.Character != 12 {
		t.Fatalf(
			"rangeForEntityDeclaration() end=%d:%d, want 1:12",
			rng.End.Line,
			rng.End.Character,
		)
	}
}

func TestDocumentSymbolRangeUsesEntityBody(t *testing.T) {
	t.Parallel()

	moduleRef := core.ModuleRef{Path: "@"}
	componentMeta := core.Meta{
		Start: core.Position{Line: 1, Column: 0},
		Stop:  core.Position{Line: 4, Column: 1},
	}

	build := src.Build{
		Modules: map[core.ModuleRef]src.Module{
			moduleRef: {
				Packages: map[string]src.Package{
					"main": {
						"main": {
							Entities: map[string]src.Entity{
								"Main": {
									Kind: src.ComponentEntity,
									Component: []src.Component{
										{
											Interface: src.Interface{IO: src.IO{}},
											Meta:      componentMeta,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	server := &Server{
		workspacePath: "/tmp/workspace",
		indexMutex:    &sync.Mutex{},
	}
	server.setBuild(build)

	docSymbolsResult, err := server.TextDocumentDocumentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: pathToURI("/tmp/workspace/main/main.neva")},
	})
	if err != nil {
		t.Fatalf("TextDocumentDocumentSymbol() error = %v", err)
	}

	docSymbols, ok := docSymbolsResult.([]protocol.DocumentSymbol)
	if !ok {
		t.Fatalf("TextDocumentDocumentSymbol() type=%T, want []protocol.DocumentSymbol", docSymbolsResult)
	}
	if len(docSymbols) != 1 {
		t.Fatalf("TextDocumentDocumentSymbol() len=%d, want 1", len(docSymbols))
	}

	symbol := docSymbols[0]
	if symbol.Range.End.Line <= symbol.SelectionRange.End.Line {
		t.Fatalf(
			"DocumentSymbol range end line=%d, selection end line=%d; want body range to extend beyond selection",
			symbol.Range.End.Line,
			symbol.SelectionRange.End.Line,
		)
	}
}

func TestDocumentSymbolFallsBackWhenBodyRangeDoesNotContainSelectionRange(t *testing.T) {
	t.Parallel()

	moduleRef := core.ModuleRef{Path: "@"}
	componentMeta := core.Meta{
		Text:  "def BadMeta",
		Start: core.Position{Line: 1, Column: 0},
		Stop:  core.Position{Line: 1, Column: 1},
	}

	build := src.Build{
		Modules: map[core.ModuleRef]src.Module{
			moduleRef: {
				Packages: map[string]src.Package{
					"main": {
						"main": {
							Entities: map[string]src.Entity{
								"Main": {
									Kind: src.ComponentEntity,
									Component: []src.Component{
										{Meta: componentMeta},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	server := &Server{
		workspacePath: "/tmp/workspace",
		indexMutex:    &sync.Mutex{},
	}
	server.setBuild(build)

	result, err := server.TextDocumentDocumentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: pathToURI("/tmp/workspace/main/main.neva")},
	})
	if err != nil {
		t.Fatalf("TextDocumentDocumentSymbol() error = %v", err)
	}
	symbols, ok := result.([]protocol.DocumentSymbol)
	if !ok {
		t.Fatalf("TextDocumentDocumentSymbol() type=%T, want []protocol.DocumentSymbol", result)
	}
	if len(symbols) != 1 {
		t.Fatalf("TextDocumentDocumentSymbol() len=%d, want 1", len(symbols))
	}
	if symbols[0].Range != symbols[0].SelectionRange {
		t.Fatalf(
			"TextDocumentDocumentSymbol() range=%+v selectionRange=%+v, expected fallback to selectionRange",
			symbols[0].Range,
			symbols[0].SelectionRange,
		)
	}
}

func TestCreateDiagnosticsRangeSanitization(t *testing.T) {
	t.Parallel()

	server := &Server{}

	tests := []struct {
		name string
		meta *core.Meta
		want protocol.Range
	}{
		{
			name: "meta_nil_uses_default_range",
			meta: nil,
			want: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 1},
			},
		},
		{
			name: "start_and_stop_zero_uses_default_range",
			meta: &core.Meta{
				Start: core.Position{Line: 0, Column: 0},
				Stop:  core.Position{Line: 0, Column: 0},
			},
			want: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 1},
			},
		},
		{
			name: "valid_start_and_stop_are_converted_to_lsp_zero_based_lines",
			meta: &core.Meta{
				Start: core.Position{Line: 3, Column: 4},
				Stop:  core.Position{Line: 3, Column: 10},
			},
			want: protocol.Range{
				Start: protocol.Position{Line: 2, Character: 4},
				End:   protocol.Position{Line: 2, Character: 10},
			},
		},
		{
			name: "stop_zero_falls_back_to_next_character_from_start",
			meta: &core.Meta{
				Start: core.Position{Line: 5, Column: 7},
				Stop:  core.Position{Line: 0, Column: 0},
			},
			want: protocol.Range{
				Start: protocol.Position{Line: 4, Character: 7},
				End:   protocol.Position{Line: 4, Character: 8},
			},
		},
		{
			name: "negative_start_values_use_default_range",
			meta: &core.Meta{
				Start: core.Position{Line: -1, Column: -3},
				Stop:  core.Position{Line: -1, Column: -1},
			},
			want: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 1},
			},
		},
		{
			name: "large_values_are_clamped_without_underflow",
			meta: &core.Meta{
				Start: core.Position{Line: math.MaxInt, Column: math.MaxInt},
				Stop:  core.Position{Line: math.MaxInt, Column: math.MaxInt},
			},
			want: protocol.Range{
				Start: protocol.Position{Line: math.MaxUint32, Character: math.MaxUint32},
				End:   protocol.Position{Line: math.MaxUint32, Character: math.MaxUint32},
			},
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			params := server.createDiagnostics(indexer.Error{
				Meta:    testCase.meta,
				Message: "boom",
			}, "file:///tmp/main.neva")

			if len(params.Diagnostics) != 1 {
				t.Fatalf("createDiagnostics() diagnostics=%d, want 1", len(params.Diagnostics))
			}
			gotRange := params.Diagnostics[0].Range
			if gotRange != testCase.want {
				t.Fatalf("createDiagnostics() range=%+v, want %+v", gotRange, testCase.want)
			}
		})
	}
}

func TestReadOnlyHandlersGracefulWhenFileMissingFromBuild(t *testing.T) {
	t.Parallel()

	server, _ := buildTestLSPServerWithSingleFile()
	missingURI := pathToURI("/tmp/workspace/main/missing.neva")

	docSymbolsResult, err := server.TextDocumentDocumentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: missingURI},
	})
	if err != nil {
		t.Fatalf("TextDocumentDocumentSymbol() error = %v", err)
	}
	docSymbols, ok := docSymbolsResult.([]protocol.DocumentSymbol)
	if !ok {
		t.Fatalf("TextDocumentDocumentSymbol() type=%T, want []protocol.DocumentSymbol", docSymbolsResult)
	}
	if len(docSymbols) != 0 {
		t.Fatalf("TextDocumentDocumentSymbol() len=%d, want 0", len(docSymbols))
	}

	tokens, err := server.TextDocumentSemanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: missingURI},
	})
	if err != nil {
		t.Fatalf("TextDocumentSemanticTokensFull() error = %v", err)
	}
	if tokens == nil {
		t.Fatalf("TextDocumentSemanticTokensFull() returned nil tokens")
	}
	if len(tokens.Data) != 0 {
		t.Fatalf("TextDocumentSemanticTokensFull() data len=%d, want 0", len(tokens.Data))
	}

	lenses, err := server.TextDocumentCodeLens(nil, &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: missingURI},
	})
	if err != nil {
		t.Fatalf("TextDocumentCodeLens() error = %v", err)
	}
	if len(lenses) != 0 {
		t.Fatalf("TextDocumentCodeLens() len=%d, want 0", len(lenses))
	}
}

func TestSemanticTokensPresentForValidDocument(t *testing.T) {
	t.Parallel()

	mainFile := strings.TrimSpace(`
pub type Id int

def Main(start int) (stop int) {
	dec Dec
	---
	:start -> dec -> :stop
}
`) + "\n"

	server, docURI, _ := buildIndexedServerWithSingleMainFile(t, mainFile)
	tokens, err := server.TextDocumentSemanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	})
	if err != nil {
		t.Fatalf("TextDocumentSemanticTokensFull() error = %v", err)
	}
	if tokens == nil {
		t.Fatal("TextDocumentSemanticTokensFull() returned nil tokens")
	}
	if len(tokens.Data) == 0 {
		t.Fatal("TextDocumentSemanticTokensFull() returned empty token payload for valid document")
	}
}

func assertCompletionKind(
	t *testing.T,
	itemsByLabel map[string]protocol.CompletionItem,
	label string,
	wantKind protocol.CompletionItemKind,
) {
	t.Helper()
	item, ok := itemsByLabel[label]
	if !ok {
		t.Fatalf("missing completion label %q", label)
	}
	if item.Kind == nil {
		t.Fatalf("completion %q kind is nil", label)
	}
	if *item.Kind != wantKind {
		t.Fatalf("completion %q kind=%v, want %v", label, *item.Kind, wantKind)
	}
}

func assertLensKinds(
	t *testing.T,
	got map[string]map[codeLensKind]struct{},
	entityName string,
	want []codeLensKind,
) {
	t.Helper()
	kinds, ok := got[entityName]
	if !ok {
		t.Fatalf("missing entity lens set for %q", entityName)
	}
	if len(kinds) != len(want) {
		t.Fatalf("lens kinds for %q count=%d, want %d", entityName, len(kinds), len(want))
	}
	for _, expectedKind := range want {
		if _, ok := kinds[expectedKind]; !ok {
			t.Fatalf("missing lens kind %q for %q", expectedKind, entityName)
		}
	}
}

func buildTestLSPServerWithSingleFile() (*Server, string) {
	moduleRef := core.ModuleRef{Path: "@"}

	// Shared type expression for interface/component ports in this test fixture.
	portType := ts.Expr{Inst: &ts.InstExpr{Ref: core.EntityRef{Name: "int"}}}
	interfaceMeta := core.Meta{Start: core.Position{Line: 1, Column: 0}, Stop: core.Position{Line: 1, Column: 7}}
	componentMeta := core.Meta{Start: core.Position{Line: 3, Column: 0}, Stop: core.Position{Line: 3, Column: 12}}
	constMeta := core.Meta{Start: core.Position{Line: 5, Column: 0}, Stop: core.Position{Line: 5, Column: 6}}

	file := src.File{
		Entities: map[string]src.Entity{
			"Greeter": {
				Kind: src.InterfaceEntity,
				Interface: src.Interface{
					IO: src.IO{
						In:  map[string]src.Port{"in": {TypeExpr: portType}},
						Out: map[string]src.Port{"out": {TypeExpr: portType}},
					},
					Meta: interfaceMeta,
				},
			},
			"HelloGreeter": {
				Kind: src.ComponentEntity,
				Component: []src.Component{
					{
						Interface: src.Interface{
							IO: src.IO{
								In:  map[string]src.Port{"in": {TypeExpr: portType}},
								Out: map[string]src.Port{"out": {TypeExpr: portType}},
							},
						},
						Meta: componentMeta,
					},
				},
			},
			"Answer": {
				Kind: src.ConstEntity,
				Const: src.Const{
					Meta: constMeta,
					Value: src.ConstValue{
						Ref: &core.EntityRef{
							Name: "Greeter",
							Meta: core.Meta{Start: core.Position{Line: 5, Column: 12}, Stop: core.Position{Line: 5, Column: 19}},
						},
					},
				},
			},
		},
	}

	build := src.Build{
		Modules: map[core.ModuleRef]src.Module{
			moduleRef: {
				Packages: map[string]src.Package{
					"main": {
						"main": file,
					},
				},
			},
		},
	}

	server := &Server{
		workspacePath: "/tmp/workspace",
		indexMutex:    &sync.Mutex{},
	}
	server.setBuild(build)
	return server, pathToURI("/tmp/workspace/main/main.neva")
}
