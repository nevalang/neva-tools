package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	neva "github.com/nevalang/neva/pkg"
	"github.com/nevalang/neva/pkg/indexer"
	"github.com/tliron/commonlog"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// TestIndexAndNotifyProblemsWorkspaceWithoutRootMainPackage ensures workspace scans
// do not require a Main package at module root (only in subpackages).
func TestIndexAndNotifyProblemsWorkspaceWithoutRootMainPackage(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writeWorkspaceTestFile(t, filepath.Join(workspace, "neva.yml"), fmt.Sprintf("neva: %s\n", neva.Version))
	writeWorkspaceTestFile(t, filepath.Join(workspace, "pkg", "main.neva"), `def Main(start any) (stop any) {
	:start -> :stop
}
`)

	idx, err := indexer.NewDefault(commonlog.GetLoggerf("neva-lsp.workspace_indexing_test"))
	if err != nil {
		t.Fatalf("create indexer: %v", err)
	}

	srv := &Server{
		workspacePath:   workspace,
		logger:          commonlog.GetLoggerf("neva-lsp.workspace_server_test"),
		indexer:         idx,
		indexMutex:      &sync.Mutex{},
		problemsMutex:   &sync.Mutex{},
		problemFiles:    make(map[string]struct{}),
		activeFileMutex: &sync.Mutex{},
	}

	diagnostics := make([]protocol.PublishDiagnosticsParams, 0)
	notify := func(method string, params any) {
		if method != protocol.ServerTextDocumentPublishDiagnostics {
			return
		}
		published, ok := params.(protocol.PublishDiagnosticsParams)
		if !ok {
			t.Fatalf("unexpected diagnostics payload type: %T", params)
		}
		diagnostics = append(diagnostics, published)
	}

	if err := srv.indexAndNotifyProblems(notify); err != nil {
		t.Fatalf("indexAndNotifyProblems: %v", err)
	}

	build, ok := srv.getBuild()
	if !ok {
		t.Fatal("expected build snapshot after indexing")
	}

	entryMod, ok := build.Modules[build.EntryModRef]
	if !ok {
		t.Fatal("expected entry module in indexed build")
	}
	if _, ok := entryMod.Packages["pkg"]; !ok {
		t.Fatalf("expected package %q in entry module; got %v", "pkg", mapsKeys(entryMod.Packages))
	}

	for _, published := range diagnostics {
		for _, diag := range published.Diagnostics {
			if diag.Message == "main package not found" {
				t.Fatalf("unexpected workspace diagnostic %q for %s", diag.Message, published.URI)
			}
		}
	}
}

func writeWorkspaceTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func mapsKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	return keys
}
