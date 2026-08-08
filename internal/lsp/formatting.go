package lsp

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf16"

	"github.com/nevalang/neva/pkg/formatter"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// TextDocumentFormatting formats the current document snapshot with Neva's
// canonical, zero-configuration formatter.
func (s *Server) TextDocumentFormatting(
	_ *glsp.Context,
	params *protocol.DocumentFormattingParams,
) ([]protocol.TextEdit, error) {
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return nil, fmt.Errorf("resolve document URI: %w", err)
	}

	text, ok := s.openDocumentTextByPath(path)
	if !ok {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read document: %w", err)
		}
		text = string(contents)
	}

	formatted, formatErr := formatter.Format([]byte(text))
	if formatErr != nil {
		return nil, formatErr
	}
	if string(formatted) == text {
		return []protocol.TextEdit{}, nil
	}

	return []protocol.TextEdit{{
		Range:   fullDocumentRange(text),
		NewText: string(formatted),
	}}, nil
}

func fullDocumentRange(text string) protocol.Range {
	line := strings.Count(text, "\n")
	lastLine := text
	if newline := strings.LastIndexByte(text, '\n'); newline >= 0 {
		lastLine = text[newline+1:]
	}

	return protocol.Range{
		Start: protocol.Position{},
		End: protocol.Position{
			Line:      uint32(line),
			Character: uint32(len(utf16.Encode([]rune(lastLine)))),
		},
	}
}
