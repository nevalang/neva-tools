package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateWorkspaceSnapshotUsesOpenDocumentTextWithoutChangingWorkspace(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, "neva.yml")
	mainPath := filepath.Join(workspace, "main", "main.neva")
	ignoredPath := filepath.Join(workspace, "notes.txt")
	writeWorkspaceTestFile(t, manifestPath, "neva: 0.38.0\n")
	writeWorkspaceTestFile(t, mainPath, "def Main(start any) (stop any) {\n\t:start -> :stop\n}\n")
	writeWorkspaceTestFile(t, ignoredPath, "not compiler input\n")

	snapshot, cleanup, err := createWorkspaceSnapshot(workspace, map[string]string{
		normalizePathForLookup(mainPath): "def Broken(start) (stop) {}\n",
	})
	if err != nil {
		t.Fatalf("createWorkspaceSnapshot() error = %v", err)
	}
	t.Cleanup(cleanup)

	assertSnapshotFile(t, filepath.Join(snapshot, "neva.yml"), "neva: 0.38.0\n")
	assertSnapshotFile(t, filepath.Join(snapshot, "main", "main.neva"), "def Broken(start) (stop) {}\n")
	if _, err := os.Stat(filepath.Join(snapshot, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("snapshot copied non-source file: err=%v", err)
	}
	assertSnapshotFile(t, mainPath, "def Main(start any) (stop any) {\n\t:start -> :stop\n}\n")
}

func TestCreateWorkspaceSnapshotRejectsEmptyWorkspace(t *testing.T) {
	t.Parallel()

	if _, _, err := createWorkspaceSnapshot("", nil); err == nil {
		t.Fatal("createWorkspaceSnapshot() error = nil, want error for empty workspace")
	}
}

func assertSnapshotFile(t *testing.T, path string, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("file %q = %q, want %q", path, got, want)
	}
}
