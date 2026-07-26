// Package viewservice defines the transport-neutral model exposed to Neva tools.
//
// It projects an analyzed AST build through github.com/nevalang/neva/pkg/view;
// LSP and the standalone HTTP host are transports over this same model.
package viewservice

import "github.com/nevalang/neva/pkg/view"

// ProgramRequest controls optional program-group filtering.
// Its zero value includes all groups.
type ProgramRequest struct {
	IncludeCurrent *bool `json:"includeCurrent,omitempty"`
	IncludeDeps    *bool `json:"includeDeps,omitempty"`
	IncludeStd     *bool `json:"includeStd,omitempty"`
}

// FileRequest identifies one projected file by stable ID.
type FileRequest struct {
	FileID string `json:"fileId"`
}

// ResolveRequest identifies a projected entity by canonical IDs.
type ResolveRequest struct {
	TargetFileID   string `json:"targetFileId"`
	TargetEntityID string `json:"targetEntityId"`
}

// ResolveResult is a canonical source-navigation target.
type ResolveResult struct {
	TargetKind     string            `json:"targetKind"`
	TargetName     string            `json:"targetName"`
	TargetFileID   string            `json:"targetFileId"`
	TargetEntityID string            `json:"targetEntityId"`
	TargetAnchor   view.SourceAnchor `json:"targetAnchor"`
}

// SearchRequest performs name search across projected entities.
type SearchRequest struct {
	Query          string   `json:"query"`
	Kinds          []string `json:"kinds,omitempty"`
	ModuleFilter   string   `json:"moduleFilter,omitempty"`
	PackageFilter  string   `json:"packageFilter,omitempty"`
	ModuleFilters  []string `json:"moduleFilters,omitempty"`
	PackageFilters []string `json:"packageFilters,omitempty"`
	Limit          int      `json:"limit,omitempty"`
}

// SearchResultItem is one search match returned to a UI client.
type SearchResultItem struct {
	Label    string            `json:"label"`
	Kind     string            `json:"kind"`
	Module   string            `json:"module"`
	Package  string            `json:"package"`
	FileID   string            `json:"fileId"`
	EntityID string            `json:"entityId"`
	Anchor   view.SourceAnchor `json:"anchor"`
}
