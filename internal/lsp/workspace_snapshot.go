package lsp

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// workspaceSnapshotForIndexing creates a short-lived source-only workspace when
// any documents are open. It lets the compiler see the editor's current text
// without modifying the user's files. A workspace without open documents can be
// indexed directly.
func (s *Server) workspaceSnapshotForIndexing() (string, func(), error) {
	overlays := s.openDocumentSnapshot()
	if len(overlays) == 0 {
		return s.workspacePath, func() {}, nil
	}

	return createWorkspaceSnapshot(s.workspacePath, overlays)
}

func (s *Server) openDocumentSnapshot() map[string]string {
	if s.openDocsMutex == nil || s.openDocs == nil {
		return nil
	}

	s.openDocsMutex.Lock()
	defer s.openDocsMutex.Unlock()
	if len(s.openDocs) == 0 {
		return nil
	}

	overlays := make(map[string]string, len(s.openDocs))
	for path, text := range s.openDocs {
		overlays[path] = text
	}
	return overlays
}

// createWorkspaceSnapshot copies only the Neva manifest and source files. The
// compiler reads precisely those files, so copying additional workspace data is
// unnecessary and would make diagnostics heavier than normal editing needs.
func createWorkspaceSnapshot(workspacePath string, overlays map[string]string) (string, func(), error) {
	if workspacePath == "" {
		return "", nil, fmt.Errorf("workspace path is empty")
	}

	workspacePath = filepath.Clean(workspacePath)
	snapshot, err := os.MkdirTemp("", "neva-lsp-overlay-*")
	if err != nil {
		return "", nil, fmt.Errorf("create workspace snapshot: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(snapshot) }

	err = filepath.WalkDir(workspacePath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !isWorkspaceSourceFile(workspacePath, path) {
			return nil
		}

		rel, err := filepath.Rel(workspacePath, path)
		if err != nil {
			return err
		}
		target := filepath.Join(snapshot, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		content, ok := overlays[normalizePathForLookup(path)]
		if !ok {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content = string(data)
		}
		return os.WriteFile(target, []byte(content), 0o600)
	})
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("copy workspace snapshot: %w", err)
	}

	return snapshot, cleanup, nil
}

func isWorkspaceSourceFile(workspacePath string, path string) bool {
	if filepath.Ext(path) == ".neva" {
		return true
	}
	if filepath.Dir(path) != workspacePath {
		return false
	}
	base := filepath.Base(path)
	return base == "neva.yml" || base == "neva.yaml"
}
