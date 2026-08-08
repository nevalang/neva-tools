package lsp

import (
	"path/filepath"
	"sync"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestTextDocumentFormattingUsesOpenDocumentSnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "main.neva")
	server := &Server{
		openDocsMutex: &sync.Mutex{},
		openDocs:      make(map[string]string),
	}
	server.setOpenDocument(pathToURI(path), "def Main(start any) (stop any) {\n:start->:stop\n}\n")

	edits, err := server.TextDocumentFormatting(nil, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: pathToURI(path)},
	})
	if err != nil {
		t.Fatalf("TextDocumentFormatting() error = %v", err)
	}

	if len(edits) != 1 {
		t.Fatalf("TextDocumentFormatting() edits = %d, want 1", len(edits))
	}
	if got, want := edits[0].NewText, "def Main(start any) (stop any) {\n\t:start -> :stop\n}\n"; got != want {
		t.Fatalf("TextDocumentFormatting() new text = %q, want %q", got, want)
	}
	if got, want := edits[0].Range.End, (protocol.Position{Line: 3, Character: 0}); got != want {
		t.Fatalf("TextDocumentFormatting() range end = %#v, want %#v", got, want)
	}
}
