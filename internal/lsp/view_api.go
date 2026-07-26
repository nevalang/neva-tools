package lsp

import "github.com/nevalang/neva-tools/internal/viewservice"

const (
	methodGetProgramView   = "neva/view/getProgram"
	methodGetFileView      = "neva/view/getFileView"
	methodResolveEntityRef = "neva/view/resolveEntityRef"
	methodSearchEntities   = "neva/view/searchEntities"
)

// Transport aliases retain the current LSP method names while the shared
// view-service contract remains independent from any transport.
type GetProgramViewRequest = viewservice.ProgramRequest
type GetFileViewRequest = viewservice.FileRequest
type ResolveEntityRefRequest = viewservice.ResolveRequest
type ResolveEntityRefResult = viewservice.ResolveResult
type SearchEntitiesRequest = viewservice.SearchRequest
type SearchEntitiesResultItem = viewservice.SearchResultItem
