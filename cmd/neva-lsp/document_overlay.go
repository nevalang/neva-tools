package main

import (
	"os"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func openDocPathFromURI(uri string) (string, bool) {
	path, err := uriToPath(uri)
	if err != nil || path == "" {
		return "", false
	}
	return normalizePathForLookup(path), true
}

func (s *Server) setOpenDocument(uri string, text string) {
	if s.openDocsMutex == nil || s.openDocs == nil {
		return
	}
	path, ok := openDocPathFromURI(uri)
	if !ok {
		return
	}

	s.openDocsMutex.Lock()
	s.openDocs[path] = text
	s.openDocsMutex.Unlock()
}

func (s *Server) deleteOpenDocument(uri string) {
	if s.openDocsMutex == nil || s.openDocs == nil {
		return
	}
	path, ok := openDocPathFromURI(uri)
	if !ok {
		return
	}

	s.openDocsMutex.Lock()
	delete(s.openDocs, path)
	s.openDocsMutex.Unlock()
}

func (s *Server) openDocumentTextByPath(path string) (string, bool) {
	if s.openDocsMutex == nil || s.openDocs == nil || path == "" {
		return "", false
	}
	normalizedPath := normalizePathForLookup(path)

	s.openDocsMutex.Lock()
	text, ok := s.openDocs[normalizedPath]
	s.openDocsMutex.Unlock()
	return text, ok
}

func (s *Server) applyOpenDocumentChanges(uri string, changes []any) {
	if s.openDocsMutex == nil || s.openDocs == nil {
		return
	}
	path, ok := openDocPathFromURI(uri)
	if !ok {
		return
	}

	s.openDocsMutex.Lock()
	current := s.openDocs[path]
	if current == "" {
		if fileBytes, err := os.ReadFile(path); err == nil {
			current = string(fileBytes)
		}
	}
	for _, rawChange := range changes {
		switch change := rawChange.(type) {
		case protocol.TextDocumentContentChangeEventWhole:
			current = change.Text
		case protocol.TextDocumentContentChangeEvent:
			if change.Range == nil {
				current = change.Text
				continue
			}
			start, okStart := offsetAtPosition(current, change.Range.Start)
			end, okEnd := offsetAtPosition(current, change.Range.End)
			if !okStart || !okEnd || start > end {
				current = change.Text
				continue
			}
			current = current[:start] + change.Text + current[end:]
		}
	}
	s.openDocs[path] = current
	s.openDocsMutex.Unlock()
}

func offsetAtPosition(text string, pos protocol.Position) (int, bool) {
	if pos.Line == 0 {
		column := int(pos.Character)
		if column < 0 || column > len(text) {
			return 0, false
		}
		return column, true
	}

	targetLine := int(pos.Line)
	line := 0
	offset := 0
	for offset < len(text) && line < targetLine {
		if text[offset] == '\n' {
			line++
		}
		offset++
	}
	if line != targetLine {
		return 0, false
	}

	column := int(pos.Character)
	lineEnd := offset
	for lineEnd < len(text) && text[lineEnd] != '\n' {
		lineEnd++
	}
	if column < 0 || offset+column > lineEnd {
		return 0, false
	}
	return offset + column, true
}

func lineAtFromText(text string, line int) (string, bool) {
	if line < 0 {
		return "", false
	}
	lines := strings.Split(text, "\n")
	if line >= len(lines) {
		return "", false
	}
	return strings.TrimSuffix(lines[line], "\r"), true
}
