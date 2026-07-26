package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nevalang/neva/pkg/view"
)

func TestQueryBoolPtr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rawQuery string
		wantNil  bool
		want     bool
	}{
		{name: "missing", rawQuery: "", wantNil: true},
		{name: "true", rawQuery: "includeCurrent=true", want: true},
		{name: "false", rawQuery: "includeCurrent=false", want: false},
		{name: "one", rawQuery: "includeCurrent=1", want: true},
		{name: "zero", rawQuery: "includeCurrent=0", want: false},
		{name: "invalid", rawQuery: "includeCurrent=maybe", wantNil: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("GET", "/api/view/program?"+tc.rawQuery, nil)
			got := queryBoolPtr(req, "includeCurrent")
			if tc.wantNil {
				if got != nil {
					t.Fatalf("queryBoolPtr()=%v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("queryBoolPtr()=nil, want non-nil")
			}
			if *got != tc.want {
				t.Fatalf("queryBoolPtr()=%v, want %v", *got, tc.want)
			}
		})
	}
}

func TestParseManifestDeps(t *testing.T) {
	t.Parallel()

	raw := `
version: "0.37.1"
deps:
  std: "0.37.1"
  github.com/example/mod: "1.2.3"
`

	deps := parseManifestDeps(raw)
	if len(deps) != 2 {
		t.Fatalf("parseManifestDeps() len=%d, want 2", len(deps))
	}
	if deps["std"] != "0.37.1" {
		t.Fatalf("parseManifestDeps() std=%q, want 0.37.1", deps["std"])
	}
	if deps["github.com/example/mod"] != "1.2.3" {
		t.Fatalf("parseManifestDeps() github.com/example/mod=%q, want 1.2.3", deps["github.com/example/mod"])
	}
}

func TestRegisterStaticUI_MissingDist_ReturnsInstruction(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("NEVA_LSP_WEB_DIST", filepath.Join(workspace, "web", "dist"))
	mux := http.NewServeMux()
	registerStaticUI(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if body := rec.Body.String(); body == "" {
		t.Fatal("expected non-empty fallback message")
	}
}

func TestRegisterStaticUI_ServesIndexAndAssets(t *testing.T) {
	workspace := t.TempDir()
	distDir := filepath.Join(workspace, "web", "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "asset.txt"), []byte("asset"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	t.Setenv("NEVA_LSP_WEB_DIST", distDir)

	mux := http.NewServeMux()
	registerStaticUI(mux)

	recIndex := httptest.NewRecorder()
	mux.ServeHTTP(recIndex, httptest.NewRequest(http.MethodGet, "/", nil))
	if recIndex.Code != http.StatusOK {
		t.Fatalf("index status=%d, want 200", recIndex.Code)
	}

	recAsset := httptest.NewRecorder()
	mux.ServeHTTP(recAsset, httptest.NewRequest(http.MethodGet, "/asset.txt", nil))
	if recAsset.Code != http.StatusOK {
		t.Fatalf("asset status=%d, want 200", recAsset.Code)
	}
	if got := recAsset.Body.String(); got != "asset" {
		t.Fatalf("asset body=%q, want asset", got)
	}
}

func TestEmbeddedWebDistFS_IsAvailableBeforeWebBuild(t *testing.T) {
	fsys, err := embeddedWebDistFS()
	if err != nil {
		t.Fatalf("embeddedWebDistFS() error: %v", err)
	}
	if _, err := fsys.Open("placeholder.txt"); err != nil {
		t.Fatalf("embedded UI placeholder open error: %v", err)
	}
}

func TestDetectWorkspaceProgramScope_PreservesProgramAndReportsEntryFiles(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "hello_world"), 0o755); err != nil {
		t.Fatalf("mkdir hello_world: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "hello_world", "main.neva"), []byte("def Main(start any) (stop any) {}"), 0o644); err != nil {
		t.Fatalf("write main.neva: %v", err)
	}

	scope := detectWorkspaceProgramScope(filepath.Join(workspace, "hello_world"))
	program := view.Program{Modules: []view.Module{
		{
			Path: "@",
			Packages: []view.Package{
				{Name: "hello_world", FileSummaries: []view.FileSummary{{ID: "module/@/package/hello_world/file/main", Path: "hello_world/main.neva"}}},
				{Name: "other", FileSummaries: []view.FileSummary{{ID: "module/@/package/other/file/main", Path: "other/main.neva"}}},
			},
		},
		{
			Path: "std",
			Packages: []view.Package{
				{Name: "fmt", FileSummaries: []view.FileSummary{{Path: "fmt/main.neva"}}},
			},
		},
	}}

	gotProgram := scope.filterCurrentModule(program)
	if len(gotProgram.Modules) != 2 {
		t.Fatalf("module count=%d, want 2", len(gotProgram.Modules))
	}
	if len(gotProgram.Modules[0].Packages) != 2 {
		t.Fatalf("current module package count=%d, want 2", len(gotProgram.Modules[0].Packages))
	}
	gotEntries := scope.entryFileIDs(program)
	if len(gotEntries) != 1 {
		t.Fatalf("entry file count=%d, want 1", len(gotEntries))
	}
	if gotEntries[0] != "module/@/package/hello_world/file/main" {
		t.Fatalf("entry file=%q, want hello_world main file", gotEntries[0])
	}
}

func TestWorkspaceProgramScope_LeavesProgramWhenNoMatches(t *testing.T) {
	t.Parallel()

	scope := workspaceProgramScope{
		enabled: true,
		allowedFilePaths: map[string]struct{}{
			"unrelated/main.neva": {},
		},
	}
	program := view.Program{Modules: []view.Module{
		{
			Path: "@",
			Packages: []view.Package{
				{Name: "main", FileSummaries: []view.FileSummary{{Path: "main/main.neva"}}},
			},
		},
	}}

	filtered := scope.filterCurrentModule(program)
	if len(filtered.Modules) != 1 || len(filtered.Modules[0].Packages) != 1 {
		t.Fatal("expected original current module packages to be preserved")
	}
}
