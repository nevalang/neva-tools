// Semantic tokens provide semantic highlighting categories beyond lexical tokenization.
// We emit declaration/reference tokens for Neva entities plus node/port address segments
// so editors can color symbols consistently across the current document.
package main

import (
	"math"
	"sort"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	src "github.com/nevalang/neva/pkg/ast"
)

// semanticTokenTypes returns the token names declared in the semantic token legend.
func semanticTokenTypes() []string {
	return []string{
		"namespace",
		"type",
		"class",
		"interface",
		"function",
		"variable",
		"property",
		"keyword",
		"constant",
	}
}

// semanticTokensLegend returns the static token schema advertised during initialize.
func semanticTokensLegend() protocol.SemanticTokensLegend {
	tokenTypes := semanticTokenTypes()
	return protocol.SemanticTokensLegend{
		TokenTypes:     tokenTypes,
		TokenModifiers: []string{},
	}
}

type semanticToken struct {
	line      int
	start     int
	length    int
	tokenType int
	modifiers int
}

// TextDocumentSemanticTokensFull returns full semantic tokens for a single Neva document.
func (s *Server) TextDocumentSemanticTokensFull(
	glspCtx *glsp.Context,
	params *protocol.SemanticTokensParams,
) (*protocol.SemanticTokens, error) {
	build, ok := s.getBuild()
	if !ok {
		return &protocol.SemanticTokens{Data: []uint32{}}, nil
	}

	fileCtx, err := s.findFile(build, params.TextDocument.URI)
	if err != nil {
		if isFileNotFoundInBuild(err) {
			s.logReadOnlyFileMiss("textDocument/semanticTokens/full", params.TextDocument.URI, err)
			return &protocol.SemanticTokens{Data: []uint32{}}, nil
		}
		return nil, err
	}

	fileText := s.fileText(fileCtx.filePath)
	fileLines := readSemanticTokenLines(fileText)
	tokens := s.collectSemanticTokens(build, fileCtx, fileLines)
	if len(tokens) == 0 && len(fileCtx.file.Entities) > 0 {
		// Fallback to metadata-only tokenization to avoid dropping all highlighting
		// when frontend metadata and source text disagree for a whole document.
		tokens = s.collectSemanticTokens(build, fileCtx, nil)
	}
	encodedTokenData := encodeSemanticTokens(tokens)
	return &protocol.SemanticTokens{Data: encodedTokenData}, nil
}

// collectSemanticTokens gathers declaration, reference, and port-address tokens from a file.
func (s *Server) collectSemanticTokens(build *src.Build, fileCtx *fileContext, fileLines []string) []semanticToken {
	typeIndex := tokenTypeIndex()
	tokens := make([]semanticToken, 0, len(fileCtx.file.Entities))
	fileText := s.fileText(fileCtx.filePath)

	for name, entity := range fileCtx.file.Entities {
		if tokenType, ok := entityTokenType(entity.Kind, typeIndex); ok {
			if declarationRange, found := rangeForEntityDeclaration(entity, name, fileCtx.filePath); found {
				if token, tokenOK := tokenFromNamedRange(declarationRange, tokenType, name, fileLines); tokenOK {
					tokens = append(tokens, token)
				}
			}
		}

		if entity.Kind == src.ComponentEntity {
			entityMeta := entity.Meta()
			for _, comp := range entity.Component {
				if entityMeta != nil && entityMeta.Start.Line > 0 {
					lineIdx := entityMeta.Start.Line - 1
					if lineText, ok := lineAtFromText(fileText, lineIdx); ok {
						if _, ok := declarationNameIndexInLine(lineText, "def", name); ok {
							for _, signaturePort := range signaturePortRangesInLine(lineText, lineIdx) {
								if token, tokenOK := tokenFromRange(signaturePort.defRange, typeIndex["property"]); tokenOK {
									tokens = append(tokens, token)
								}
							}
						}
					}
				}

				for _, conn := range comp.Net {
					tokens = append(tokens, collectPortTokens(conn, typeIndex, fileLines, fileText)...)
				}
			}
		}
	}

	refs := collectRefsInFile(fileCtx.file)
	for _, ref := range refs {
		resolved, ok := s.resolveEntityRef(build, fileCtx, ref.ref)
		if !ok {
			continue
		}
		tokenType, ok := entityTokenType(resolved.entity.Kind, typeIndex)
		if !ok {
			continue
		}
		refRange := entityRefNameRange(ref.meta, ref.ref)
		if token, tokenOK := tokenFromNamedRange(refRange, tokenType, ref.ref.Name, fileLines); tokenOK {
			tokens = append(tokens, token)
		}
	}

	// LSP expects monotonically ordered tokens before delta encoding.
	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].line == tokens[j].line {
			return tokens[i].start < tokens[j].start
		}
		return tokens[i].line < tokens[j].line
	})
	tokens = dropOverlappingSemanticTokens(tokens)

	return tokens
}

func readSemanticTokenLines(fileText string) []string {
	if fileText == "" {
		return nil
	}
	return strings.Split(fileText, "\n")
}

func tokenFromRange(r protocol.Range, tokenType int) (semanticToken, bool) {
	if r.Start.Line != r.End.Line || r.End.Character <= r.Start.Character {
		return semanticToken{}, false
	}
	return semanticToken{
		line:      int(r.Start.Line),
		start:     int(r.Start.Character),
		length:    int(r.End.Character - r.Start.Character),
		tokenType: tokenType,
		modifiers: 0,
	}, true
}

func tokenFromNamedRange(
	r protocol.Range,
	tokenType int,
	expectedName string,
	fileLines []string,
) (semanticToken, bool) {
	token, ok := tokenFromRange(r, tokenType)
	if !ok {
		return semanticToken{}, false
	}

	if expectedName == "" {
		return token, true
	}
	if len(fileLines) == 0 {
		return token, true
	}
	if rangeMatchesFileText(r, expectedName, fileLines) {
		return token, true
	}

	adjustedRange, ok := realignRangeToLineIdentifier(r, expectedName, fileLines)
	if !ok {
		return semanticToken{}, false
	}
	adjustedToken, ok := tokenFromRange(adjustedRange, tokenType)
	if !ok {
		return semanticToken{}, false
	}

	return adjustedToken, true
}

func rangeMatchesFileText(r protocol.Range, expected string, lines []string) bool {
	if r.Start.Line != r.End.Line {
		return false
	}
	lineIdx := int(r.Start.Line)
	if lineIdx < 0 || lineIdx >= len(lines) {
		return false
	}
	start := int(r.Start.Character)
	end := int(r.End.Character)
	if start < 0 || end <= start {
		return false
	}

	line := lines[lineIdx]
	if end > len(line) {
		return false
	}

	return line[start:end] == expected
}

func realignRangeToLineIdentifier(r protocol.Range, expected string, lines []string) (protocol.Range, bool) {
	if r.Start.Line != r.End.Line || expected == "" {
		return protocol.Range{}, false
	}
	lineIdx := int(r.Start.Line)
	if lineIdx < 0 || lineIdx >= len(lines) {
		return protocol.Range{}, false
	}

	line := lines[lineIdx]
	if line == "" {
		return protocol.Range{}, false
	}

	startHint := int(r.Start.Character)
	if startHint < 0 {
		startHint = 0
	}
	if startHint > len(line) {
		startHint = len(line)
	}

	windowStart := startHint - 64
	if windowStart < 0 {
		windowStart = 0
	}
	windowEnd := startHint + len(expected) + 64
	if windowEnd > len(line) {
		windowEnd = len(line)
	}
	segment := line[windowStart:windowEnd]
	if segment == "" {
		return protocol.Range{}, false
	}

	bestStart := -1
	searchOffset := 0
	for searchOffset <= len(segment) {
		relative := strings.Index(segment[searchOffset:], expected)
		if relative < 0 {
			break
		}
		candidate := windowStart + searchOffset + relative
		leftBoundary := candidate == 0 || !isIdentifierByte(line[candidate-1])
		right := candidate + len(expected)
		rightBoundary := right >= len(line) || !isIdentifierByte(line[right])
		if leftBoundary && rightBoundary {
			if bestStart == -1 || absInt(candidate-startHint) < absInt(bestStart-startHint) {
				bestStart = candidate
			}
		}

		searchOffset += relative + 1
		if searchOffset >= len(segment) {
			break
		}
	}

	if bestStart < 0 {
		return protocol.Range{}, false
	}
	return spanRange(lineIdx, bestStart, bestStart+len(expected)), true
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// tokenTypeIndex maps legend token names to numeric indices.
func tokenTypeIndex() map[string]int {
	tokenTypes := semanticTokenTypes()
	index := make(map[string]int, len(tokenTypes))
	for i, t := range tokenTypes {
		index[t] = i
	}
	return index
}

// entityTokenType maps Neva entity kinds to semantic token categories.
func entityTokenType(kind src.EntityKind, index map[string]int) (int, bool) {
	switch kind {
	case src.TypeEntity:
		return index["type"], true
	case src.InterfaceEntity:
		return index["interface"], true
	case src.ComponentEntity:
		return index["function"], true
	case src.ConstEntity:
		return index["constant"], true
	default:
		return 0, false
	}
}

// collectPortTokens walks a connection tree and tokenizes node/port address segments.
func collectPortTokens(conn src.Connection, index map[string]int, fileLines []string, fileText string) []semanticToken {
	var tokens []semanticToken
	for _, sender := range conn.Senders {
		if sender.PortAddr != nil {
			tokens = append(tokens, portAddrTokens(*sender.PortAddr, index, fileLines, fileText)...)
		}
	}
	for _, receiver := range conn.Receivers {
		if receiver.PortAddr != nil {
			tokens = append(tokens, portAddrTokens(*receiver.PortAddr, index, fileLines, fileText)...)
		}
		if receiver.ChainedConnection != nil {
			tokens = append(tokens, collectPortTokens(*receiver.ChainedConnection, index, fileLines, fileText)...)
		}
	}
	return tokens
}

// portAddrTokens emits separate tokens for node names and port names inside an address.
func portAddrTokens(addr src.PortAddr, index map[string]int, fileLines []string, fileText string) []semanticToken {
	var tokens []semanticToken

	if addr.Node != "" && !strings.HasPrefix(addr.Node, ":") {
		if nodeRange, ok := rangeForNodeInPortAddr(addr); ok {
			if token, tokenOK := tokenFromNamedRange(nodeRange, index["variable"], addr.Node, fileLines); tokenOK {
				tokens = append(tokens, token)
			}
		}
	}

	if addr.Port != "" {
		if portRange, ok := rangeForPortInPortAddr(addr); ok {
			if expandedRange, expanded := normalizeIdentifierRangeInText(portRange, fileText); expanded {
				portRange = expandedRange
			}
			if token, tokenOK := tokenFromRange(portRange, index["property"]); tokenOK {
				tokens = append(tokens, token)
			}
		}
	}

	return tokens
}

func dropOverlappingSemanticTokens(tokens []semanticToken) []semanticToken {
	if len(tokens) <= 1 {
		return tokens
	}

	filtered := make([]semanticToken, 0, len(tokens))
	for _, token := range tokens {
		if token.length <= 0 {
			continue
		}
		if len(filtered) == 0 {
			filtered = append(filtered, token)
			continue
		}

		lastIdx := len(filtered) - 1
		prev := filtered[lastIdx]
		prevEnd := prev.start + prev.length

		if token.line != prev.line || token.start >= prevEnd {
			filtered = append(filtered, token)
			continue
		}

		// When two semantic tokens overlap, keep the one with the larger span.
		// Overlapping tokens are invalid for VS Code and may disable highlighting.
		if token.length > prev.length {
			filtered[lastIdx] = token
		}
	}

	return filtered
}

// encodeSemanticTokens converts absolute tokens to LSP delta-encoded payload format.
func encodeSemanticTokens(tokens []semanticToken) []uint32 {
	encodedData := make([]uint32, 0, len(tokens)*5)
	lastLine := 0
	lastStart := 0

	for _, token := range tokens {
		deltaLine := token.line - lastLine
		deltaStart := token.start
		if deltaLine == 0 {
			deltaStart = token.start - lastStart
		}

		deltaLine32, ok := intToUint32(deltaLine)
		if !ok {
			continue
		}
		deltaStart32, ok := intToUint32(deltaStart)
		if !ok {
			continue
		}
		length32, ok := intToUint32(token.length)
		if !ok {
			continue
		}
		tokenType32, ok := intToUint32(token.tokenType)
		if !ok {
			continue
		}
		modifiers32, ok := intToUint32(token.modifiers)
		if !ok {
			continue
		}

		encodedData = append(encodedData, deltaLine32, deltaStart32, length32, tokenType32, modifiers32)

		lastLine = token.line
		lastStart = token.start
	}

	return encodedData
}

// intToUint32 safely converts a non-negative int that fits into uint32.
func intToUint32(value int) (uint32, bool) {
	if value < 0 || value > math.MaxUint32 {
		return 0, false
	}
	// #nosec G115 -- range is checked above before conversion.
	return uint32(value), true
}
