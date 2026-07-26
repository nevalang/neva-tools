package main

import (
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestRangeMatchesFileText(t *testing.T) {
	t.Parallel()

	lines := []string{
		"import {",
		"def Main(start any) (stop any) {",
	}

	ok := rangeMatchesFileText(
		protocol.Range{
			Start: protocol.Position{Line: 1, Character: 4},
			End:   protocol.Position{Line: 1, Character: 8},
		},
		"Main",
		lines,
	)
	if !ok {
		t.Fatal("rangeMatchesFileText() = false, want true for exact match")
	}
}

func TestTokenFromNamedRangeDropsMismatchedText(t *testing.T) {
	t.Parallel()

	lines := []string{
		"import {",
	}

	_, ok := tokenFromNamedRange(
		protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 3},
		},
		1,
		"any",
		lines,
	)
	if ok {
		t.Fatal("tokenFromNamedRange() = ok for mismatched source text, want false")
	}
}

func TestSemanticTokensNoOverlapForPortAddresses(t *testing.T) {
	t.Parallel()

	mainFile := strings.TrimSpace(`
def Echo(data any) (res any) {
	:data -> :res
}

def Main(start any) (stop any) {
	echo Echo
	---
	:start -> echo:data
	echo:res -> :stop
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
		t.Fatal("TextDocumentSemanticTokensFull() returned nil")
	}
	if len(tokens.Data) == 0 {
		t.Fatal("TextDocumentSemanticTokensFull() returned empty payload")
	}

	absolute := decodeAbsoluteTokens(tokens.Data)

	for i := 1; i < len(absolute); i++ {
		prev := absolute[i-1]
		curr := absolute[i]
		if curr.line == prev.line && curr.start < prev.start+prev.length {
			t.Fatalf(
				"overlapping semantic tokens: prev=%+v curr=%+v payload=%v",
				prev,
				curr,
				tokens.Data,
			)
		}
	}
}

func TestSemanticTokensIncludeFullComponentPortNameWithDigits(t *testing.T) {
	t.Parallel()

	mainFile := strings.TrimSpace(`
def Main(start any) (stop123 any) {
	:start -> :stop123
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
		t.Fatal("TextDocumentSemanticTokensFull() returned nil")
	}

	absolute := decodeAbsoluteTokens(tokens.Data)
	if !hasAbsoluteToken(absolute, 0, 21, len("stop123")) {
		t.Fatalf("missing semantic token for full stop123 declaration; payload=%v", tokens.Data)
	}
	if !hasAbsoluteToken(absolute, 0, 29, len("any")) {
		t.Fatalf("missing semantic token for output type any; payload=%v", tokens.Data)
	}
}

func TestSemanticTokensTrackUnsavedPortRenameFromDidChange(t *testing.T) {
	t.Parallel()

	mainFile := strings.TrimSpace(`
def Main(start any) (stop any) {
	:start -> :stop
}
`) + "\n"
	changedFile := strings.ReplaceAll(mainFile, "stop", "stop123")

	server, docURI, _ := buildIndexedServerWithSingleMainFile(t, mainFile)

	if err := server.TextDocumentDidOpen(nil, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:  docURI,
			Text: mainFile,
		},
	}); err != nil {
		t.Fatalf("TextDocumentDidOpen() error = %v", err)
	}
	if err := server.TextDocumentDidChange(nil, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: docURI},
			Version:                2,
		},
		ContentChanges: []any{
			protocol.TextDocumentContentChangeEventWhole{Text: changedFile},
		},
	}); err != nil {
		t.Fatalf("TextDocumentDidChange() error = %v", err)
	}

	tokens, err := server.TextDocumentSemanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	})
	if err != nil {
		t.Fatalf("TextDocumentSemanticTokensFull() error = %v", err)
	}
	if tokens == nil {
		t.Fatal("TextDocumentSemanticTokensFull() returned nil")
	}

	absolute := decodeAbsoluteTokens(tokens.Data)
	if !hasAbsoluteToken(absolute, 0, 21, len("stop123")) {
		t.Fatalf("missing semantic token for unsaved stop123 declaration; payload=%v", tokens.Data)
	}
	if !hasAbsoluteToken(absolute, 1, 12, len("stop123")) {
		t.Fatalf("missing semantic token for unsaved :stop123 network usage; payload=%v", tokens.Data)
	}
	anyPos := positionForNth(t, changedFile, "stop123 any", 0, len("stop123 "))
	if !hasAbsoluteToken(absolute, int(anyPos.Line), int(anyPos.Character), len("any")) {
		t.Fatalf("missing semantic token for unsaved output type any; payload=%v", tokens.Data)
	}
}

func TestSemanticTokensTrackUnsavedPortRenameFromDidChangeNonPrefix(t *testing.T) {
	t.Parallel()

	mainFile := strings.TrimSpace(`
def Main(start any) (stop any) {
	:start -> :stop
}
`) + "\n"
	changedFile := strings.ReplaceAll(mainFile, "stop", "asd")

	server, docURI, _ := buildIndexedServerWithSingleMainFile(t, mainFile)

	if err := server.TextDocumentDidOpen(nil, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:  docURI,
			Text: mainFile,
		},
	}); err != nil {
		t.Fatalf("TextDocumentDidOpen() error = %v", err)
	}
	if err := server.TextDocumentDidChange(nil, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: docURI},
			Version:                2,
		},
		ContentChanges: []any{
			protocol.TextDocumentContentChangeEventWhole{Text: changedFile},
		},
	}); err != nil {
		t.Fatalf("TextDocumentDidChange() error = %v", err)
	}

	tokens, err := server.TextDocumentSemanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	})
	if err != nil {
		t.Fatalf("TextDocumentSemanticTokensFull() error = %v", err)
	}
	if tokens == nil {
		t.Fatal("TextDocumentSemanticTokensFull() returned nil")
	}

	absolute := decodeAbsoluteTokens(tokens.Data)
	asdDeclPos := positionForNth(t, changedFile, "asd any", 0, 0)
	if !hasAbsoluteToken(absolute, int(asdDeclPos.Line), int(asdDeclPos.Character), len("asd")) {
		t.Fatalf("missing semantic token for unsaved asd declaration; payload=%v", tokens.Data)
	}
	asdRefPos := positionForNth(t, changedFile, ":asd", 0, 1)
	if !hasAbsoluteToken(absolute, int(asdRefPos.Line), int(asdRefPos.Character), len("asd")) {
		t.Fatalf("missing semantic token for unsaved :asd network usage; payload=%v", tokens.Data)
	}
	anyPos := positionForNth(t, changedFile, "asd any", 0, len("asd "))
	if !hasAbsoluteToken(absolute, int(anyPos.Line), int(anyPos.Character), len("any")) {
		t.Fatalf("missing semantic token for unsaved output type any; payload=%v", tokens.Data)
	}
}

type absoluteToken struct {
	line   int
	start  int
	length int
}

func decodeAbsoluteTokens(data []uint32) []absoluteToken {
	absolute := make([]absoluteToken, 0, len(data)/5)
	line := 0
	start := 0
	for i := 0; i+4 < len(data); i += 5 {
		deltaLine := int(data[i])
		deltaStart := int(data[i+1])
		length := int(data[i+2])

		line += deltaLine
		if deltaLine == 0 {
			start += deltaStart
		} else {
			start = deltaStart
		}

		absolute = append(absolute, absoluteToken{
			line:   line,
			start:  start,
			length: length,
		})
	}
	return absolute
}

func hasAbsoluteToken(tokens []absoluteToken, line int, start int, length int) bool {
	for _, token := range tokens {
		if token.line == line && token.start == start && token.length == length {
			return true
		}
	}
	return false
}
