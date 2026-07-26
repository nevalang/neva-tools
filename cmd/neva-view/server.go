package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/nevalang/neva-tools/internal/viewservice"
	"github.com/nevalang/neva/pkg/ast"
	"github.com/nevalang/neva/pkg/indexer"
	"github.com/nevalang/neva/pkg/view"
	"github.com/tliron/commonlog"
	"gopkg.in/yaml.v3"
)

// Config describes one standalone visual-editor server.
type viewConfig struct {
	WorkspacePath string
	ListenAddr    string
	OpenBrowser   bool
	UI            fs.FS
}

// Run scans a workspace once and hosts the common visual-editor UI and API.
func runView(logger commonlog.Logger, config viewConfig) error {
	idx, err := indexer.NewDefault(logger)
	if err != nil {
		return fmt.Errorf("create indexer: %w", err)
	}
	build, found, scanErr := idx.FullScan(context.Background(), config.WorkspacePath)
	if scanErr != nil {
		return fmt.Errorf("scan workspace: %w", scanErr)
	}
	if !found {
		return errors.New("no Neva module found in workspace")
	}
	mux := NewMux(config.WorkspacePath, &build, config.UI)
	url := "http://" + config.ListenAddr
	logger.Info("Neva View running", "url", url)
	if config.OpenBrowser {
		_ = openBrowser(url)
	}
	return (&http.Server{Addr: config.ListenAddr, Handler: mux}).ListenAndServe()
}

// NewMux is exposed for HTTP-level tests and embedding in other hosts.
func NewMux(workspacePath string, build *ast.Build, ui fs.FS) *http.ServeMux {
	mux := http.NewServeMux()
	registerAPI(mux, build, readManifest(workspacePath), detectScope(workspacePath))
	registerUI(mux, ui)
	return mux
}

func registerAPI(mux *http.ServeMux, build *ast.Build, manifest manifestView, scope workspaceScope) {
	mux.HandleFunc("/api/view/program", func(w http.ResponseWriter, req *http.Request) {
		params := viewservice.ProgramRequest{IncludeCurrent: queryBool(req, "includeCurrent"), IncludeDeps: queryBool(req, "includeDeps"), IncludeStd: queryBool(req, "includeStd")}
		program := viewservice.Program(*build, params)
		writeJSON(w, struct {
			view.Program
			EntryFileIDs []string `json:"entryFileIds,omitempty"`
		}{Program: program, EntryFileIDs: scope.entryFileIDs(program)})
	})
	mux.HandleFunc("/api/view/file", func(w http.ResponseWriter, req *http.Request) {
		result, err := viewservice.File(*build, viewservice.FileRequest{FileID: req.URL.Query().Get("id")})
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, result)
	})
	mux.HandleFunc("/api/view/search", func(w http.ResponseWriter, req *http.Request) {
		result, err := viewservice.Search(*build, viewservice.SearchRequest{Query: strings.TrimSpace(req.URL.Query().Get("q")), Kinds: req.URL.Query()["kind"], ModuleFilters: req.URL.Query()["module"], PackageFilters: req.URL.Query()["package"], Limit: 100})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, result)
	})
	mux.HandleFunc("/api/view/manifest", func(w http.ResponseWriter, req *http.Request) {
		module := strings.TrimSpace(req.URL.Query().Get("module"))
		switch module {
		case "", "@":
			writeJSON(w, manifest)
		case "std":
			writeJSON(w, manifestView{Path: "std/neva.yml", Deps: map[string]string{}})
		default:
			writeJSON(w, manifestView{Path: module + "/neva.yml", Deps: map[string]string{}})
		}
	})
	mux.HandleFunc("/api/view/resolve", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload viewservice.ResolveRequest
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		result, err := viewservice.Resolve(*build, payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, result)
	})
}

func registerUI(mux *http.ServeMux, ui fs.FS) {
	fileServer := http.FileServer(http.FS(ui))
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			asset := strings.TrimPrefix(filepath.Clean(req.URL.Path), string(filepath.Separator))
			if _, err := fs.Stat(ui, asset); err == nil {
				fileServer.ServeHTTP(w, req)
				return
			}
		}
		if _, err := fs.Stat(ui, "index.html"); err != nil {
			http.Error(w, "Neva View UI is not embedded. Build it with make web-build before installing neva-view.", http.StatusServiceUnavailable)
			return
		}
		http.ServeFileFS(w, req, ui, "index.html")
	})
}

type manifestView struct {
	Path    string            `json:"path"`
	Raw     string            `json:"raw"`
	Deps    map[string]string `json:"deps"`
	Present bool              `json:"present"`
}

func readManifest(workspace string) manifestView {
	for _, path := range []string{filepath.Join(workspace, "neva.yml"), filepath.Join(workspace, "neva.yaml")} {
		content, err := os.ReadFile(path)
		if err == nil {
			raw := string(content)
			return manifestView{Path: path, Raw: raw, Deps: parseDeps(raw), Present: true}
		}
	}
	return manifestView{Deps: map[string]string{}}
}
func parseDeps(raw string) map[string]string {
	var parsed struct {
		Deps map[string]string `yaml:"deps"`
	}
	_ = yaml.Unmarshal([]byte(raw), &parsed)
	if parsed.Deps == nil {
		return map[string]string{}
	}
	return parsed.Deps
}

type workspaceScope struct{ allowed map[string]struct{} }

func detectScope(workspace string) workspaceScope {
	allowed := map[string]struct{}{}
	_ = filepath.WalkDir(workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".neva" {
			return nil
		}
		rel, err := filepath.Rel(workspace, path)
		if err == nil {
			rel = filepath.ToSlash(filepath.Clean(rel))
			allowed[rel] = struct{}{}
			allowed[filepath.ToSlash(filepath.Join(filepath.Base(workspace), rel))] = struct{}{}
		}
		return nil
	})
	return workspaceScope{allowed: allowed}
}
func (s workspaceScope) entryFileIDs(program view.Program) []string {
	result := []string{}
	for _, module := range program.Modules {
		if module.Path != "@" {
			continue
		}
		for _, pkg := range module.Packages {
			for _, file := range pkg.FileSummaries {
				if _, ok := s.allowed[filepath.ToSlash(filepath.Clean(file.Path))]; ok {
					result = append(result, file.ID)
				}
			}
		}
	}
	slices.Sort(result)
	return result
}
func queryBool(req *http.Request, key string) *bool {
	switch req.URL.Query().Get(key) {
	case "1", "true", "TRUE", "True":
		v := true
		return &v
	case "0", "false", "FALSE", "False":
		v := false
		return &v
	default:
		return nil
	}
}
func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
