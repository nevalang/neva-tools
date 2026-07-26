package main

import (
	src "github.com/nevalang/neva/pkg/ast"
)

type LegacyGetFileViewRequest struct {
	WorkspaceURI LegacyURI `json:"workspaceUri"`
	Document     struct {
		URI      LegacyURI `json:"uri"`
		FileName string    `json:"fileName"`
	} `json:"document"`
}

type LegacyURI struct {
	Path   string `json:"path"`
	FSPath string `json:"fsPath"`
}

type LegacyGetFileViewResponse struct {
	File  src.File `json:"file"`
	Extra Extra    `json:"extra"` // info that is not presented in the file but needed for rendering
}

type Extra struct {
	NodesPorts map[string]map[string]src.Interface `json:"nodesPorts"` // flows -> nodes -> interface
}

// LEGACY NOTE:
// `resolve_file` request/response structures are intentionally kept only as
// historical reference during neva/view migration.
// The legacy handler is no longer registered in LSP dispatch.
