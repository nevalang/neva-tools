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

	ast "github.com/nevalang/neva/pkg/ast"
	"github.com/nevalang/neva/pkg/indexer"
	"github.com/nevalang/neva/pkg/view"
	"github.com/tliron/commonlog"
	"gopkg.in/yaml.v3"
)

func runStandaloneView(logger commonlog.Logger, workspacePath string, listenAddr string, openBrowserFlag bool) error {
	idx, err := indexer.NewDefault(logger)
	if err != nil {
		return fmt.Errorf("create indexer: %w", err)
	}

	build, found, scanErr := idx.FullScan(context.Background(), workspacePath)
	if scanErr != nil {
		return fmt.Errorf("scan workspace: %w", scanErr)
	}
	if !found {
		return errors.New("no Neva module found in workspace")
	}

	manifestCurrent := readManifestView(workspacePath)
	scope := detectWorkspaceProgramScope(workspacePath)
	mux := buildStandaloneMux(workspacePath, &build, manifestCurrent, scope)

	url := "http://" + listenAddr
	logger.Info("standalone visual view running", "url", url)
	if openBrowserFlag {
		_ = openBrowser(url)
	}

	server := &http.Server{Addr: listenAddr, Handler: mux}
	return server.ListenAndServe()
}

func buildStandaloneMux(workspacePath string, build *ast.Build, manifestCurrent manifestView, scope workspaceProgramScope) *http.ServeMux {
	mux := http.NewServeMux()
	registerViewAPI(mux, build, manifestCurrent, scope)
	registerStaticUI(mux)
	return mux
}

func registerViewAPI(mux *http.ServeMux, build *ast.Build, manifestCurrent manifestView, scope workspaceProgramScope) {
	mux.HandleFunc("/api/view/program", func(w http.ResponseWriter, req *http.Request) {
		params := GetProgramViewRequest{
			IncludeCurrent: queryBoolPtr(req, "includeCurrent"),
			IncludeDeps:    queryBoolPtr(req, "includeDeps"),
			IncludeStd:     queryBoolPtr(req, "includeStd"),
		}
		program := scope.filterCurrentModule(view.ProjectProgram(*build))
		writeJSON(w, filterProgramModules(program, params))
	})

	mux.HandleFunc("/api/view/file", func(w http.ResponseWriter, req *http.Request) {
		fileID := req.URL.Query().Get("id")
		if fileID == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		fileView, ok := projectFileView(*build, fileID)
		if !ok {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		writeJSON(w, fileView)
	})

	mux.HandleFunc("/api/view/search", func(w http.ResponseWriter, req *http.Request) {
		q := strings.TrimSpace(req.URL.Query().Get("q"))
		kinds := req.URL.Query()["kind"]
		modules := req.URL.Query()["module"]
		packages := req.URL.Query()["package"]
		resultAny, err := searchEntitiesInBuild(build, SearchEntitiesRequest{
			Query:          q,
			Kinds:          kinds,
			ModuleFilters:  modules,
			PackageFilters: packages,
			Limit:          100,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, resultAny)
	})

	mux.HandleFunc("/api/view/manifest", func(w http.ResponseWriter, req *http.Request) {
		modulePath := strings.TrimSpace(req.URL.Query().Get("module"))
		switch modulePath {
		case "", "@":
			writeJSON(w, manifestCurrent)
		case "std":
			writeJSON(w, manifestView{Path: "std/neva.yml", Present: false, Deps: map[string]string{}})
		default:
			writeJSON(w, manifestView{Path: modulePath + "/neva.yml", Present: false, Deps: map[string]string{}})
		}
	})

	mux.HandleFunc("/api/view/resolve", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload ResolveEntityRefRequest
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		result, err := resolveEntityRefInBuild(build, payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, result)
	})
}

func registerStaticUI(mux *http.ServeMux) {
	var uiFS fs.FS
	if override := strings.TrimSpace(os.Getenv("NEVA_LSP_WEB_DIST")); override != "" {
		uiFS = os.DirFS(override)
	} else {
		embedded, err := embeddedWebDistFS()
		if err != nil {
			panic(fmt.Errorf("load embedded ui: %w", err))
		}
		uiFS = embedded
	}
	fileServer := http.FileServer(http.FS(uiFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			assetRelPath := strings.TrimPrefix(filepath.Clean(req.URL.Path), string(filepath.Separator))
			if _, err := fs.Stat(uiFS, assetRelPath); err == nil {
				fileServer.ServeHTTP(w, req)
				return
			}
		}

		if _, err := fs.Stat(uiFS, "index.html"); err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Standalone UI is not available. Build web/dist before go install, or set NEVA_LSP_WEB_DIST."))
			return
		}
		http.ServeFileFS(w, req, uiFS, "index.html")
	})
}

func resolveEntityRefInBuild(build *ast.Build, params ResolveEntityRefRequest) (ResolveEntityRefResult, error) {
	if params.TargetFileID == "" {
		return ResolveEntityRefResult{}, errors.New("targetFileId is required")
	}
	if params.TargetEntityID == "" {
		return ResolveEntityRefResult{}, errors.New("targetEntityId is required")
	}

	fileView, found := view.ProjectFileByID(*build, params.TargetFileID)
	if !found {
		return ResolveEntityRefResult{}, fmt.Errorf("file not found: %s", params.TargetFileID)
	}

	result, found := findEntityInFile(fileView, params.TargetEntityID)
	if !found {
		return ResolveEntityRefResult{}, fmt.Errorf("entity not found: %s", params.TargetEntityID)
	}
	return result, nil
}

func searchEntitiesInBuild(build *ast.Build, params SearchEntitiesRequest) ([]SearchEntitiesResultItem, error) {
	query := strings.TrimSpace(params.Query)
	if query == "" {
		return []SearchEntitiesResultItem{}, nil
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}
	allowedKinds := map[string]struct{}{}
	for _, kind := range params.Kinds {
		kind = strings.ToLower(strings.TrimSpace(kind))
		if kind != "" {
			allowedKinds[kind] = struct{}{}
		}
	}

	moduleFilters := normalizeFilters(params.ModuleFilters, params.ModuleFilter)
	packageFilters := normalizeFilters(params.PackageFilters, params.PackageFilter)

	program := view.ProjectProgram(*build)
	results := make([]SearchEntitiesResultItem, 0, limit)

	for _, module := range program.Modules {
		if len(moduleFilters) > 0 && !isInSet(moduleFilters, module.Path) {
			continue
		}
		for _, pkg := range module.Packages {
			qualifiedPackage := module.Path + "/" + pkg.Name
			if len(packageFilters) > 0 && !isInSet(packageFilters, qualifiedPackage) {
				continue
			}
			for _, fileSummary := range pkg.FileSummaries {
				if len(results) >= limit {
					return results, nil
				}
				fileView, found := view.ProjectFileByID(*build, fileSummary.ID)
				if !found {
					continue
				}
				appendEntityMatches(&results, fileView, module.Path, pkg.Name, strings.ToLower(query), allowedKinds, limit)
				if len(results) >= limit {
					return results, nil
				}
			}
		}
	}

	return results, nil
}

type manifestView struct {
	Path    string            `json:"path"`
	Raw     string            `json:"raw"`
	Deps    map[string]string `json:"deps"`
	Present bool              `json:"present"`
}

type workspaceProgramScope struct {
	enabled          bool
	allowedFilePaths map[string]struct{}
}

func detectWorkspaceProgramScope(workspacePath string) workspaceProgramScope {
	filePaths := map[string]struct{}{}
	_ = filepath.WalkDir(workspacePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".neva" {
			return nil
		}

		rel, relErr := filepath.Rel(workspacePath, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		filePaths[rel] = struct{}{}
		filePaths[filepath.ToSlash(filepath.Join(filepath.Base(workspacePath), rel))] = struct{}{}
		return nil
	})

	return workspaceProgramScope{
		enabled:          len(filePaths) > 0,
		allowedFilePaths: filePaths,
	}
}

func (s workspaceProgramScope) filterCurrentModule(program view.Program) view.Program {
	if !s.enabled {
		return program
	}

	filtered := view.Program{Modules: make([]view.Module, 0, len(program.Modules))}
	for _, module := range program.Modules {
		if classifyModule(module.Path) != "current" {
			filtered.Modules = append(filtered.Modules, module)
			continue
		}

		moduleCopy := module
		moduleCopy.Packages = slices.DeleteFunc(append([]view.Package(nil), module.Packages...), func(pkg view.Package) bool {
			for _, fileSummary := range pkg.FileSummaries {
				if _, ok := s.allowedFilePaths[filepath.ToSlash(filepath.Clean(fileSummary.Path))]; ok {
					return false
				}
			}
			return true
		})

		if len(moduleCopy.Packages) == 0 {
			filtered.Modules = append(filtered.Modules, module)
			continue
		}
		filtered.Modules = append(filtered.Modules, moduleCopy)
	}

	return filtered
}

func readManifestView(workspacePath string) manifestView {
	candidates := []string{
		filepath.Join(workspacePath, "neva.yml"),
		filepath.Join(workspacePath, "neva.yaml"),
	}
	for _, path := range candidates {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		raw := string(content)
		return manifestView{
			Path:    path,
			Raw:     raw,
			Deps:    parseManifestDeps(raw),
			Present: true,
		}
	}
	return manifestView{Present: false, Deps: map[string]string{}}
}

func parseManifestDeps(raw string) map[string]string {
	type manifest struct {
		Deps map[string]string `yaml:"deps"`
	}
	var m manifest
	_ = yamlUnmarshal([]byte(raw), &m)
	if m.Deps == nil {
		return map[string]string{}
	}
	return m.Deps
}

var yamlUnmarshal = func(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		panic(err)
	}
}

func queryBoolPtr(req *http.Request, key string) *bool {
	raw := req.URL.Query().Get(key)
	if raw == "" {
		return nil
	}
	switch raw {
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
