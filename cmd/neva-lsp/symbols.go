// Package server symbol helpers implement definition/reference/rename/hover/symbol lookups.
// The core idea is: resolve cursor position to an entity reference, then map it back to
// declaration and usage ranges across all workspace-resolved files.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	src "github.com/nevalang/neva/pkg/ast"
	"github.com/nevalang/neva/pkg/core"
	ts "github.com/nevalang/neva/pkg/typesystem"
)

var errFileNotFoundInBuild = errors.New("file not found in build")

type fileContext struct {
	file        src.File
	moduleRef   core.ModuleRef
	packageName string
	fileName    string
	filePath    string
}

// resolvedEntity stores a fully-resolved entity target used by multiple LSP handlers.
type resolvedEntity struct {
	moduleRef   core.ModuleRef
	packageName string
	name        string
	filePath    string
	entity      src.Entity
}

// refOccurrence records one textual reference with the source metadata span.
type refOccurrence struct {
	ref  core.EntityRef
	meta core.Meta
}

// componentContext captures the innermost component for position-sensitive completions.
type componentContext struct {
	name      string
	component src.Component
}

type entityRefSegment string

const (
	entityRefSegmentName    entityRefSegment = "name"
	entityRefSegmentPackage entityRefSegment = "package"
)

type entityRefHit struct {
	ref     core.EntityRef
	meta    core.Meta
	segment entityRefSegment
}

type importAliasHit struct {
	alias string
	imp   src.Import
	rng   protocol.Range
}

type nodeHit struct {
	componentName string
	component     src.Component
	nodeName      string
	node          src.Node
	rng           protocol.Range
}

type portHit struct {
	name       string
	owner      string
	direction  string
	typeExpr   string
	defPath    string
	defRange   protocol.Range
	hoverRange protocol.Range
}

// uriToPath converts a file URI to a local path.
func uriToPath(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "file" {
		return parsed.Path, nil
	}
	return uri, nil
}

// pathToURI converts a local path to a file URI.
func pathToURI(path string) string {
	u := url.URL{Scheme: "file", Path: path}
	return u.String()
}

// forEachWorkspaceFile iterates files that can be mapped to a workspace path.
func (s *Server) forEachWorkspaceFile(build *src.Build, visit func(fileContext) bool) {
	for modRef, mod := range build.Modules {
		for pkgName, pkg := range mod.Packages {
			for fileName, file := range pkg {
				loc := core.Location{
					ModRef:   modRef,
					Package:  pkgName,
					Filename: fileName,
				}
				filePath := s.pathForLocation(loc)
				if filePath == "" {
					continue
				}
				if !visit(fileContext{
					file:        file,
					moduleRef:   modRef,
					packageName: pkgName,
					fileName:    fileName,
					filePath:    filePath,
				}) {
					return
				}
			}
		}
	}
}

// findFile resolves an LSP document URI to compiler file context.
func (s *Server) findFile(build *src.Build, uri string) (*fileContext, error) {
	filePath, err := uriToPath(uri)
	if err != nil {
		return nil, err
	}
	filePath = normalizePathForLookup(filePath)

	var matchedCtx *fileContext
	s.forEachWorkspaceFile(build, func(candidate fileContext) bool {
		if normalizePathForLookup(candidate.filePath) == filePath {
			matchedCtx = &candidate
			return false
		}
		return true
	})
	if matchedCtx != nil {
		return matchedCtx, nil
	}

	return nil, fmt.Errorf("%w: %s", errFileNotFoundInBuild, uri)
}

func normalizePathForLookup(path string) string {
	cleanPath := filepath.Clean(path)
	resolvedPath, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		return cleanPath
	}
	return filepath.Clean(resolvedPath)
}

func (s *Server) fileText(path string) string {
	if path == "" {
		return ""
	}
	if text, ok := s.openDocumentTextByPath(path); ok {
		return text
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

func rangesEqual(a protocol.Range, b protocol.Range) bool {
	return a.Start.Line == b.Start.Line &&
		a.Start.Character == b.Start.Character &&
		a.End.Line == b.End.Line &&
		a.End.Character == b.End.Character
}

func samePortDeclaration(a *portHit, b *portHit) bool {
	if a == nil || b == nil {
		return false
	}
	if a.name != b.name || a.direction != b.direction {
		return false
	}
	if normalizePathForLookup(a.defPath) != normalizePathForLookup(b.defPath) {
		return false
	}
	return rangesEqual(a.defRange, b.defRange)
}

func appendUniqueTextEdit(edits []protocol.TextEdit, candidate protocol.TextEdit) []protocol.TextEdit {
	for _, edit := range edits {
		if edit.NewText == candidate.NewText && rangesEqual(edit.Range, candidate.Range) {
			return edits
		}
	}
	return append(edits, candidate)
}

func isFileNotFoundInBuild(err error) bool {
	return errors.Is(err, errFileNotFoundInBuild)
}

func (s *Server) logReadOnlyFileMiss(feature string, uri string, err error) {
	if s.logger == nil {
		return
	}
	s.logger.Info("read-only request skipped for file missing from index", "feature", feature, "uri", uri, "err", err)
}

// pathForLocation maps compiler source locations to workspace file paths.
func (s *Server) pathForLocation(loc core.Location) string {
	if loc.Filename == "" {
		return ""
	}

	filename := loc.Filename + ".neva"
	switch loc.ModRef.Path {
	case "@":
		return filepath.Clean(filepath.Join(s.workspacePath, loc.Package, filename))
	case "std":
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Clean(filepath.Join(home, "neva", "std", loc.Package, filename))
	default:
		depsRoot, err := depsModuleRootPath(loc.ModRef)
		if err != nil {
			return ""
		}
		return filepath.Clean(filepath.Join(depsRoot, loc.Package, filename))
	}
}

func depsModuleRootPath(modRef core.ModuleRef) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if modRef.Path == "" {
		return "", errors.New("empty dependency module path")
	}

	depsDir := filepath.Join(home, "neva", "deps")
	if modRef.Version != "" {
		return filepath.Join(depsDir, fmt.Sprintf("%s_%s", modRef.Path, modRef.Version)), nil
	}

	pattern := filepath.Join(depsDir, fmt.Sprintf("%s_*", modRef.Path))
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("dependency path not found for %s", modRef.Path)
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

// lspToCorePosition converts zero-based LSP coordinates to core.Position.
func lspToCorePosition(pos protocol.Position) core.Position {
	return core.Position{
		Line:   int(pos.Line) + 1,
		Column: int(pos.Character),
	}
}

// metaContains reports whether a position is inside a metadata span.
func metaContains(meta core.Meta, pos core.Position) bool {
	if meta.Start.Line == 0 && meta.Stop.Line == 0 {
		return false
	}

	if pos.Line < meta.Start.Line || pos.Line > meta.Stop.Line {
		return false
	}
	if pos.Line == meta.Start.Line && pos.Column < meta.Start.Column {
		return false
	}
	if pos.Line == meta.Stop.Line && pos.Column > meta.Stop.Column {
		return false
	}
	return true
}

func spanRange(line int, start int, end int) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{
			Line:      clampToUint32(line),
			Character: clampToUint32(start),
		},
		End: protocol.Position{
			Line:      clampToUint32(line),
			Character: clampToUint32(end),
		},
	}
}

func rangeIsValid(r protocol.Range) bool {
	if r.Start.Line > r.End.Line {
		return false
	}
	if r.Start.Line == r.End.Line && r.Start.Character >= r.End.Character {
		return false
	}
	return true
}

func positionInRange(pos core.Position, r protocol.Range) bool {
	line := int(r.Start.Line) + 1
	if pos.Line != line {
		return false
	}
	start := int(r.Start.Character)
	end := int(r.End.Character)
	return pos.Column >= start && pos.Column < end
}

// rangeForName builds an LSP range for an entity name inside a metadata span.
func rangeForName(meta core.Meta, name string, nameOffset int) protocol.Range {
	startLine := meta.Start.Line - 1
	startChar := meta.Start.Column + nameOffset

	if meta.Start.Line > 0 && meta.Text != "" {
		offset := findIdentifierInMetaText(meta.Text, name, nameOffset)
		if offset >= 0 {
			start := metaPositionAtOffset(meta, offset)
			end := protocol.Position{
				Line:      start.Line,
				Character: clampToUint32(int(start.Character) + len(name)),
			}
			return protocol.Range{Start: start, End: end}
		}
	}

	endChar := startChar + len(name)
	return protocol.Range{
		Start: protocol.Position{Line: clampToUint32(startLine), Character: clampToUint32(startChar)},
		End:   protocol.Position{Line: clampToUint32(startLine), Character: clampToUint32(endChar)},
	}
}

// clampToUint32 converts int to uint32 while preserving valid bounds for LSP positions.
func clampToUint32(value int) uint32 {
	if value <= 0 {
		return 0
	}
	if value > math.MaxUint32 {
		return math.MaxUint32
	}
	// #nosec G115 -- value range is bounded above before conversion.
	return uint32(value)
}

// nameOffsetForRef returns the offset for qualified references like `pkg.Name`.
func nameOffsetForRef(meta core.Meta, ref core.EntityRef) int {
	if ref.Pkg == "" {
		return 0
	}
	if meta.Text == "" {
		return len(ref.Pkg) + 1
	}
	lastDot := strings.LastIndex(meta.Text, ".")
	if lastDot == -1 {
		return len(ref.Pkg) + 1
	}
	return lastDot + 1
}

func entityRefNameRange(meta core.Meta, ref core.EntityRef) protocol.Range {
	offset := nameOffsetForRef(meta, ref)
	return rangeForName(meta, ref.Name, offset)
}

func isIdentifierByte(char byte) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') ||
		char == '_'
}

func isIdentifierStartByte(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

func isValidIdentifierName(name string) bool {
	if name == "" || !isIdentifierStartByte(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !isIdentifierByte(name[i]) {
			return false
		}
	}
	return true
}

func findIdentifierInMetaText(text string, name string, startOffset int) int {
	if text == "" || name == "" {
		return -1
	}
	if startOffset < 0 {
		startOffset = 0
	}
	if startOffset >= len(text) {
		startOffset = 0
	}
	for _, offset := range []int{startOffset, 0} {
		index := offset
		for {
			rel := strings.Index(text[index:], name)
			if rel < 0 {
				break
			}
			pos := index + rel
			leftBoundary := pos == 0 || !isIdentifierByte(text[pos-1])
			rightPos := pos + len(name)
			rightBoundary := rightPos >= len(text) || !isIdentifierByte(text[rightPos])
			if leftBoundary && rightBoundary {
				return pos
			}
			index = pos + 1
			if index >= len(text) {
				break
			}
		}
	}
	return -1
}

func metaPositionAtOffset(meta core.Meta, offset int) protocol.Position {
	line := meta.Start.Line - 1
	column := meta.Start.Column
	if offset <= 0 || meta.Text == "" {
		return protocol.Position{
			Line:      clampToUint32(line),
			Character: clampToUint32(column),
		}
	}
	if offset > len(meta.Text) {
		offset = len(meta.Text)
	}

	for i := 0; i < offset; i++ {
		switch meta.Text[i] {
		case '\n':
			line++
			column = 0
		case '\r':
			// Skip CR; LF in CRLF will advance line.
		default:
			column++
		}
	}
	return protocol.Position{
		Line:      clampToUint32(line),
		Character: clampToUint32(column),
	}
}

func entityDeclKeyword(kind src.EntityKind) string {
	switch kind {
	case src.TypeEntity:
		return "type"
	case src.ConstEntity:
		return "const"
	case src.InterfaceEntity:
		return "interface"
	case src.ComponentEntity:
		return "def"
	default:
		return ""
	}
}

func declarationNameIndexInLine(line string, keyword string, name string) (int, bool) {
	if line == "" || keyword == "" || name == "" {
		return 0, false
	}

	start := 0
	for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	if start >= len(line) {
		return 0, false
	}
	if strings.HasPrefix(line[start:], "//") || strings.HasPrefix(line[start:], "#") {
		return 0, false
	}

	i := start
	if strings.HasPrefix(line[i:], "pub") {
		next := i + len("pub")
		if next < len(line) && (line[next] == ' ' || line[next] == '\t') {
			i = next
			for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
				i++
			}
		}
	}

	if !strings.HasPrefix(line[i:], keyword) {
		return 0, false
	}
	i += len(keyword)
	if i >= len(line) || (line[i] != ' ' && line[i] != '\t') {
		return 0, false
	}
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	startName := i
	for i < len(line) && isIdentifierByte(line[i]) {
		i++
	}
	if startName == i {
		return 0, false
	}
	return startName, line[startName:i] == name
}

func declarationNameOffsetInText(text string, keyword string, name string) (int, bool) {
	if text == "" || keyword == "" || name == "" {
		return 0, false
	}

	offset := 0
	lineStart := 0
	for lineStart <= len(text) {
		lineEnd := strings.IndexByte(text[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(text) - lineStart
		}
		line := text[lineStart : lineStart+lineEnd]
		line = strings.TrimSuffix(line, "\r")
		if nameIndex, ok := declarationNameIndexInLine(line, keyword, name); ok {
			return offset + nameIndex, true
		}

		if lineStart+lineEnd >= len(text) {
			break
		}
		lineLen := lineEnd + 1
		offset += lineLen
		lineStart += lineLen
	}
	return 0, false
}

func declarationNameRangeInFile(filePath string, keyword string, name string) (protocol.Range, bool) {
	if filePath == "" || keyword == "" || name == "" {
		return protocol.Range{}, false
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return protocol.Range{}, false
	}

	line := 0
	start := 0
	for start <= len(content) {
		endRel := bytes.IndexByte(content[start:], '\n')
		if endRel < 0 {
			endRel = len(content) - start
		}
		lineBytes := content[start : start+endRel]
		lineBytes = bytes.TrimSuffix(lineBytes, []byte{'\r'})
		if nameIndex, ok := declarationNameIndexInLine(string(lineBytes), keyword, name); ok {
			return spanRange(line, nameIndex, nameIndex+len(name)), true
		}
		if start+endRel >= len(content) {
			break
		}
		start += endRel + 1
		line++
	}
	return protocol.Range{}, false
}

func rangeForEntityDeclaration(entity src.Entity, name string, filePath string) (protocol.Range, bool) {
	keyword := entityDeclKeyword(entity.Kind)
	if keyword == "" || name == "" {
		return protocol.Range{}, false
	}

	meta := entity.Meta()
	if meta != nil {
		if offset, ok := declarationNameOffsetInText(meta.Text, keyword, name); ok {
			rng := rangeForName(*meta, name, offset)
			if rangeIsValid(rng) {
				return rng, true
			}
		}
		if meta.Start.Line > 0 {
			rng := rangeForName(*meta, name, 0)
			if rangeIsValid(rng) {
				return rng, true
			}
		}
	}

	rng, ok := declarationNameRangeInFile(filePath, keyword, name)
	return rng, ok
}

func rangeForEntityBody(meta core.Meta) (protocol.Range, bool) {
	if meta.Start.Line <= 0 {
		return protocol.Range{}, false
	}

	start := protocol.Position{
		Line:      clampToUint32(meta.Start.Line - 1),
		Character: clampToUint32(meta.Start.Column),
	}

	endLine := meta.Stop.Line
	endColumn := meta.Stop.Column
	if endLine <= 0 {
		endLine = meta.Start.Line
		endColumn = meta.Start.Column + 1
	}
	end := protocol.Position{
		Line:      clampToUint32(endLine - 1),
		Character: clampToUint32(endColumn),
	}
	if end.Line < start.Line || (end.Line == start.Line && end.Character <= start.Character) {
		end = nextCharacterPosition(start)
	}

	return protocol.Range{Start: start, End: end}, true
}

func entityRefPkgRange(meta core.Meta, ref core.EntityRef) (protocol.Range, bool) {
	if ref.Pkg == "" || meta.Start.Line == 0 {
		return protocol.Range{}, false
	}
	line := meta.Start.Line - 1
	start := meta.Start.Column
	end := start + len(ref.Pkg)
	return spanRange(line, start, end), true
}

// resolveEntityRef resolves a reference from file context to a concrete entity.
func (s *Server) resolveEntityRef(build *src.Build, ctx *fileContext, ref core.EntityRef) (*resolvedEntity, bool) {
	modRef := ctx.moduleRef
	pkgName := ctx.packageName

	if ref.Pkg != "" {
		imp, ok := ctx.file.Imports[ref.Pkg]
		if !ok {
			return nil, false
		}
		pkgName = imp.Package
		for mod := range build.Modules {
			if mod.Path == imp.Module {
				modRef = mod
				break
			}
		}
	}

	mod, ok := build.Modules[modRef]
	if !ok {
		return nil, false
	}

	entity, filename, err := mod.Entity(core.EntityRef{Pkg: pkgName, Name: ref.Name})
	if err != nil {
		if ref.Pkg == "" {
			builtinTarget, found := s.resolveBuiltinEntityRef(build, ref.Name)
			if found {
				return builtinTarget, true
			}
		}
		return nil, false
	}

	loc := entity.Meta().Location
	if loc.Filename == "" {
		loc = core.Location{ModRef: modRef, Package: pkgName, Filename: filename}
	}

	return &resolvedEntity{
		moduleRef:   modRef,
		packageName: pkgName,
		name:        ref.Name,
		entity:      entity,
		filePath:    s.pathForLocation(loc),
	}, true
}

func (s *Server) resolveBuiltinEntityRef(build *src.Build, name string) (*resolvedEntity, bool) {
	const builtinPackageName = "builtin"
	for moduleRef, module := range build.Modules {
		if moduleRef.Path != "std" {
			continue
		}

		entity, filename, err := module.Entity(core.EntityRef{Pkg: builtinPackageName, Name: name})
		if err != nil {
			continue
		}

		location := entity.Meta().Location
		if location.Filename == "" {
			location = core.Location{
				ModRef:   moduleRef,
				Package:  builtinPackageName,
				Filename: filename,
			}
		}

		return &resolvedEntity{
			moduleRef:   moduleRef,
			packageName: builtinPackageName,
			name:        name,
			entity:      entity,
			filePath:    s.pathForLocation(location),
		}, true
	}
	return nil, false
}

func (s *Server) resolveImportPackage(
	build *src.Build,
	ctx *fileContext,
	alias string,
) (core.ModuleRef, src.Import, src.Package, string, bool) {
	imp, ok := ctx.file.Imports[alias]
	if !ok {
		return core.ModuleRef{}, src.Import{}, nil, "", false
	}

	modRef := core.ModuleRef{Path: imp.Module}
	for mod := range build.Modules {
		if mod.Path == imp.Module {
			modRef = mod
			break
		}
	}

	mod, ok := build.Modules[modRef]
	if !ok {
		return core.ModuleRef{}, src.Import{}, nil, "", false
	}

	pkg, ok := mod.Packages[imp.Package]
	if !ok {
		return core.ModuleRef{}, src.Import{}, nil, "", false
	}

	fileName := preferredPackageFileName(imp.Package, pkg)
	if fileName == "" {
		return core.ModuleRef{}, src.Import{}, nil, "", false
	}

	filePath := s.pathForLocation(core.Location{
		ModRef:   modRef,
		Package:  imp.Package,
		Filename: fileName,
	})

	return modRef, imp, pkg, filePath, true
}

func preferredPackageFileName(pkgName string, pkg src.Package) string {
	if len(pkg) == 0 {
		return ""
	}

	baseName := path.Base(pkgName)
	if _, ok := pkg[baseName]; ok {
		return baseName
	}

	fileNames := make([]string, 0, len(pkg))
	for fileName := range pkg {
		fileNames = append(fileNames, fileName)
	}
	sort.Strings(fileNames)

	return fileNames[0]
}

// findEntityDefinitionAtPosition finds declarations whose name range contains the cursor.
func (s *Server) findEntityDefinitionAtPosition(ctx *fileContext, pos core.Position) (string, *src.Entity, bool) {
	for name, entity := range ctx.file.Entities {
		defRange, ok := rangeForEntityDeclaration(entity, name, ctx.filePath)
		if !ok {
			continue
		}
		start := core.Position{Line: int(defRange.Start.Line) + 1, Column: int(defRange.Start.Character)}
		stop := core.Position{Line: int(defRange.End.Line) + 1, Column: int(defRange.End.Character)}
		if metaContains(core.Meta{Start: start, Stop: stop}, pos) {
			return name, &entity, true
		}
	}
	return "", nil, false
}

// collectRefsInFile recursively walks a file AST and collects entity references.
//
//nolint:gocyclo // Traversal intentionally handles all relevant AST variants in one pass.
func collectRefsInFile(file src.File) []refOccurrence {
	var refs []refOccurrence

	addRef := func(ref core.EntityRef) {
		// Parser can synthesize implicit refs (for example unconstrained generic params -> `any`)
		// without concrete source coordinates. They must not participate in position-based LSP features.
		if ref.Meta.Start.Line <= 0 {
			return
		}
		refs = append(refs, refOccurrence{ref: ref, meta: ref.Meta})
	}

	// Traverse type expressions to catch generic instantiations and nested literals.
	var visitTypeExpr func(expr ts.Expr)
	visitTypeExpr = func(expr ts.Expr) {
		if expr.Inst != nil {
			addRef(expr.Inst.Ref)
			for _, arg := range expr.Inst.Args {
				visitTypeExpr(arg)
			}
			return
		}
		if expr.Lit != nil {
			switch expr.Lit.Type() {
			case ts.EmptyLitType:
				// No nested references in empty literals.
			case ts.StructLitType:
				for _, field := range expr.Lit.Struct {
					visitTypeExpr(field)
				}
			case ts.UnionLitType:
				for _, tag := range expr.Lit.Union {
					if tag != nil {
						visitTypeExpr(*tag)
					}
				}
			}
		}
	}

	// Traverse const/message literals to include references in nested values.
	var visitConstValue func(val src.ConstValue)
	var visitMsgLiteral func(msg src.MsgLiteral)

	visitConstValue = func(val src.ConstValue) {
		if val.Ref != nil {
			addRef(*val.Ref)
			return
		}
		if val.Message != nil {
			visitMsgLiteral(*val.Message)
		}
	}

	visitMsgLiteral = func(msg src.MsgLiteral) {
		if msg.Union != nil {
			addRef(msg.Union.EntityRef)
			if msg.Union.Data != nil {
				visitConstValue(*msg.Union.Data)
			}
		}
		for _, item := range msg.List {
			visitConstValue(item)
		}
		for _, item := range msg.DictOrStruct {
			visitConstValue(item)
		}
	}

	// Traverse interfaces, nodes, and connections for references in wiring and DI args.
	visitInterface := func(iface src.Interface) {
		for _, port := range iface.IO.In {
			visitTypeExpr(port.TypeExpr)
		}
		for _, port := range iface.IO.Out {
			visitTypeExpr(port.TypeExpr)
		}
	}

	var visitNode func(node src.Node)
	visitNode = func(node src.Node) {
		addRef(node.EntityRef)
		for _, arg := range node.TypeArgs {
			visitTypeExpr(arg)
		}
		for _, di := range node.DIArgs {
			visitNode(di)
		}
	}

	var visitConnection func(conn src.Connection)
	visitConnection = func(conn src.Connection) {
		for _, sender := range conn.Senders {
			if sender.Const != nil {
				visitConstValue(sender.Const.Value)
			}
		}
		for _, receiver := range conn.Receivers {
			if receiver.ChainedConnection != nil {
				visitConnection(*receiver.ChainedConnection)
			}
		}
	}

	// Visit every entity body and collect references from relevant subtrees.
	for _, entity := range file.Entities {
		switch entity.Kind {
		case src.TypeEntity:
			if entity.Type.BodyExpr != nil {
				visitTypeExpr(*entity.Type.BodyExpr)
			}
			for _, param := range entity.Type.Params {
				visitTypeExpr(param.Constr)
			}
		case src.ConstEntity:
			visitTypeExpr(entity.Const.TypeExpr)
			visitConstValue(entity.Const.Value)
		case src.InterfaceEntity:
			visitInterface(entity.Interface)
		case src.ComponentEntity:
			for _, comp := range entity.Component {
				visitInterface(comp.Interface)
				for _, node := range comp.Nodes {
					visitNode(node)
				}
				for _, conn := range comp.Net {
					visitConnection(conn)
				}
			}
		}
	}

	return refs
}

// findEntityRefAtPosition finds the reference segment (entity name or package alias) under the cursor.
func (s *Server) findEntityRefAtPosition(ctx *fileContext, pos core.Position) (*entityRefHit, bool) {
	refs := collectRefsInFile(ctx.file)
	for _, ref := range refs {
		nameRange := entityRefNameRange(ref.meta, ref.ref)
		if positionInRange(pos, nameRange) {
			return &entityRefHit{
				ref:     ref.ref,
				meta:    ref.meta,
				segment: entityRefSegmentName,
			}, true
		}

		pkgRange, ok := entityRefPkgRange(ref.meta, ref.ref)
		if ok && positionInRange(pos, pkgRange) {
			return &entityRefHit{
				ref:     ref.ref,
				meta:    ref.meta,
				segment: entityRefSegmentPackage,
			}, true
		}
	}
	return nil, false
}

func findImportAliasAtPosition(ctx *fileContext, pos core.Position) (*importAliasHit, bool) {
	for alias, imp := range ctx.file.Imports {
		rng, ok := rangeForImportAlias(alias, imp.Meta)
		if !ok {
			continue
		}
		if positionInRange(pos, rng) {
			return &importAliasHit{
				alias: alias,
				imp:   imp,
				rng:   rng,
			}, true
		}
	}
	return nil, false
}

func rangeForImportAlias(alias string, meta core.Meta) (protocol.Range, bool) {
	if alias == "" || meta.Start.Line == 0 {
		return protocol.Range{}, false
	}

	start := meta.Start.Column
	if meta.Text != "" {
		if idx := strings.Index(meta.Text, alias); idx >= 0 {
			start = meta.Start.Column + idx
		}
	}

	return spanRange(meta.Start.Line-1, start, start+len(alias)), true
}

func forEachComponent(file src.File, visit func(componentName string, component src.Component) bool) {
	for entityName, entity := range file.Entities {
		if entity.Kind != src.ComponentEntity {
			continue
		}
		for _, component := range entity.Component {
			if !visit(entityName, component) {
				return
			}
		}
	}
}

func forEachConnectionPortAddr(
	conn src.Connection,
	visit func(addr src.PortAddr, isSender bool) bool,
) bool {
	for _, sender := range conn.Senders {
		if sender.PortAddr != nil && !visit(*sender.PortAddr, true) {
			return false
		}
	}
	for _, receiver := range conn.Receivers {
		if receiver.PortAddr != nil && !visit(*receiver.PortAddr, false) {
			return false
		}
		if receiver.ChainedConnection != nil && !forEachConnectionPortAddr(*receiver.ChainedConnection, visit) {
			return false
		}
	}
	return true
}

func rangeForNodeInPortAddr(addr src.PortAddr) (protocol.Range, bool) {
	if addr.Node == "" || addr.Meta.Start.Line == 0 {
		return protocol.Range{}, false
	}
	if !strings.HasPrefix(addr.Meta.Text, addr.Node) {
		return protocol.Range{}, false
	}
	start := addr.Meta.Start.Column
	end := start + len(addr.Node)
	return spanRange(addr.Meta.Start.Line-1, start, end), true
}

func rangeForPortInPortAddr(addr src.PortAddr) (protocol.Range, bool) {
	if addr.Port == "" || addr.Meta.Start.Line == 0 {
		return protocol.Range{}, false
	}

	idx := strings.Index(addr.Meta.Text, ":"+addr.Port)
	switch {
	case idx >= 0:
	case strings.HasPrefix(addr.Meta.Text, ":"+addr.Port):
		idx = 0
	case strings.HasPrefix(addr.Meta.Text, ":"):
		idx = 0
	default:
		return protocol.Range{}, false
	}

	start := addr.Meta.Start.Column + idx + 1
	end := start + len(addr.Port)
	return spanRange(addr.Meta.Start.Line-1, start, end), true
}

func normalizeIdentifierRangeInText(r protocol.Range, fileText string) (protocol.Range, bool) {
	if r.Start.Line != r.End.Line || fileText == "" {
		return protocol.Range{}, false
	}
	lineText, ok := lineAtFromText(fileText, int(r.Start.Line))
	if !ok || lineText == "" {
		return protocol.Range{}, false
	}

	start := int(r.Start.Character)
	end := int(r.End.Character)
	if start < 0 || start >= len(lineText) {
		return protocol.Range{}, false
	}
	if end < start {
		end = start
	}
	if end > len(lineText) {
		end = len(lineText)
	}

	for start > 0 && isIdentifierByte(lineText[start-1]) {
		start--
	}
	for end < len(lineText) && isIdentifierByte(lineText[end]) {
		end++
	}

	if end <= start {
		return protocol.Range{}, false
	}
	if !isIdentifierByte(lineText[start]) {
		return protocol.Range{}, false
	}

	return spanRange(int(r.Start.Line), start, end), true
}

func rangeWithLeadingColon(r protocol.Range, fileText string) protocol.Range {
	if fileText == "" || r.Start.Line != r.End.Line || r.Start.Character == 0 {
		return r
	}

	lineText, ok := lineAtFromText(fileText, int(r.Start.Line))
	if !ok {
		return r
	}
	start := int(r.Start.Character)
	if start <= 0 || start > len(lineText) {
		return r
	}
	if lineText[start-1] != ':' {
		return r
	}

	return spanRange(int(r.Start.Line), start-1, int(r.End.Character))
}

func identifierTextInRange(r protocol.Range, fileText string) (string, bool) {
	if r.Start.Line != r.End.Line || fileText == "" {
		return "", false
	}
	lineText, ok := lineAtFromText(fileText, int(r.Start.Line))
	if !ok {
		return "", false
	}

	start := int(r.Start.Character)
	end := int(r.End.Character)
	if start < 0 || end <= start || end > len(lineText) {
		return "", false
	}
	identifier := lineText[start:end]
	if identifier == "" || !isIdentifierStartByte(identifier[0]) {
		return "", false
	}
	for i := 1; i < len(identifier); i++ {
		if !isIdentifierByte(identifier[i]) {
			return "", false
		}
	}
	return identifier, true
}

func identifierRangeAtColumn(line string, column int) (int, int, bool) {
	if line == "" {
		return 0, 0, false
	}
	if column < 0 {
		column = 0
	}
	if column > len(line) {
		column = len(line)
	}

	index := column
	if index == len(line) {
		index--
	}
	if index < 0 {
		return 0, 0, false
	}
	if !isIdentifierByte(line[index]) {
		if index == 0 || !isIdentifierByte(line[index-1]) {
			return 0, 0, false
		}
		index--
	}

	start := index
	for start > 0 && isIdentifierByte(line[start-1]) {
		start--
	}
	end := index + 1
	for end < len(line) && isIdentifierByte(line[end]) {
		end++
	}
	if end <= start {
		return 0, 0, false
	}
	return start, end, true
}

func matchingParenIndex(line string, openIndex int) (int, bool) {
	if openIndex < 0 || openIndex >= len(line) || line[openIndex] != '(' {
		return 0, false
	}
	depth := 0
	for i := openIndex; i < len(line); i++ {
		switch line[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func signaturePortRangesInLine(line string, lineIdx int) []portHit {
	inOpen := strings.IndexByte(line, '(')
	inClose, ok := matchingParenIndex(line, inOpen)
	if !ok {
		return nil
	}

	outOpenRel := strings.IndexByte(line[inClose+1:], '(')
	if outOpenRel < 0 {
		return nil
	}
	outOpen := inClose + 1 + outOpenRel
	outClose, ok := matchingParenIndex(line, outOpen)
	if !ok {
		return nil
	}

	collectGroupPorts := func(groupStart int, groupEnd int, direction string) []portHit {
		hits := make([]portHit, 0, 4)
		entryStart := groupStart
		angleDepth := 0
		parenDepth := 0
		bracketDepth := 0
		braceDepth := 0
		for i := groupStart; i <= groupEnd; i++ {
			isBoundary := i == groupEnd
			if !isBoundary {
				switch line[i] {
				case '<':
					angleDepth++
				case '>':
					if angleDepth > 0 {
						angleDepth--
					}
				case '(':
					parenDepth++
				case ')':
					if parenDepth > 0 {
						parenDepth--
					}
				case '[':
					bracketDepth++
				case ']':
					if bracketDepth > 0 {
						bracketDepth--
					}
				case '{':
					braceDepth++
				case '}':
					if braceDepth > 0 {
						braceDepth--
					}
				}
			}

			if !isBoundary || angleDepth != 0 || parenDepth != 0 || bracketDepth != 0 || braceDepth != 0 {
				continue
			}
			if line[i] != ',' && i != groupEnd {
				continue
			}

			entryEnd := i
			if entryStart < entryEnd {
				nameStart := entryStart
				for nameStart < entryEnd && (line[nameStart] == ' ' || line[nameStart] == '\t') {
					nameStart++
				}
				nameEnd := nameStart
				for nameEnd < entryEnd && isIdentifierByte(line[nameEnd]) {
					nameEnd++
				}
				if nameEnd > nameStart {
					rng := spanRange(lineIdx, nameStart, nameEnd)
					hits = append(hits, portHit{
						name:       line[nameStart:nameEnd],
						direction:  direction,
						defRange:   rng,
						hoverRange: rng,
					})
				}
			}

			entryStart = i + 1
		}
		return hits
	}

	hits := collectGroupPorts(inOpen+1, inClose, "in")
	hits = append(hits, collectGroupPorts(outOpen+1, outClose, "out")...)
	return hits
}

func signaturePortHitAtPosition(
	fileText string,
	componentName string,
	componentMeta core.Meta,
	pos core.Position,
) (*portHit, bool) {
	if fileText == "" || componentName == "" || componentMeta.Start.Line <= 0 || pos.Line != componentMeta.Start.Line {
		return nil, false
	}

	lineIdx := pos.Line - 1
	lineText, ok := lineAtFromText(fileText, lineIdx)
	if !ok {
		return nil, false
	}
	if _, ok := declarationNameIndexInLine(lineText, "def", componentName); !ok {
		return nil, false
	}
	start, end, ok := identifierRangeAtColumn(lineText, pos.Column)
	if !ok {
		return nil, false
	}

	for _, candidate := range signaturePortRangesInLine(lineText, lineIdx) {
		if int(candidate.defRange.Start.Character) == start && int(candidate.defRange.End.Character) == end {
			candidate.owner = componentName
			return &candidate, true
		}
	}
	return nil, false
}

func isComponentBoundaryNode(node string) bool {
	return node == "in" || node == "out"
}

func rangeForPortName(port src.Port, name string) (protocol.Range, bool) {
	if name == "" || port.Meta.Start.Line == 0 {
		return protocol.Range{}, false
	}
	offset := 0
	if port.IsArray && strings.HasPrefix(port.Meta.Text, "[") {
		offset = 1
	}
	return rangeForName(port.Meta, name, offset), true
}

func rangeForPortNameWithFallback(
	port src.Port,
	name string,
	filePath string,
	fileText string,
	ownerMeta *core.Meta,
	ownerName string,
) (protocol.Range, bool) {
	if rng, ok := rangeForPortName(port, name); ok {
		return rng, true
	}
	if name == "" {
		return protocol.Range{}, false
	}

	if ownerMeta != nil && ownerMeta.Start.Line > 0 {
		lineText, ok := lineAtFromText(fileText, ownerMeta.Start.Line-1)
		if !ok && filePath != "" {
			var err error
			lineText, err = readLineAt(filePath, ownerMeta.Start.Line-1)
			ok = err == nil
		}
		if ok {
			if idx := findIdentifierInMetaText(lineText, name, 0); idx >= 0 {
				return spanRange(ownerMeta.Start.Line-1, idx, idx+len(name)), true
			}
		}
	}

	if ownerName != "" {
		if rng, ok := rangeForComponentPortNameInDeclaration(filePath, fileText, ownerName, name); ok {
			return rng, true
		}
	}

	return protocol.Range{}, false
}

func rangeForComponentPortNameInDeclaration(
	filePath string,
	fileText string,
	componentName string,
	portName string,
) (protocol.Range, bool) {
	if componentName == "" || portName == "" {
		return protocol.Range{}, false
	}

	if fileText == "" && filePath != "" {
		fileBytes, err := os.ReadFile(filePath)
		if err != nil {
			return protocol.Range{}, false
		}
		fileText = string(fileBytes)
	}
	if fileText == "" {
		return protocol.Range{}, false
	}

	lines := strings.Split(fileText, "\n")
	for lineIdx, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\r")
		if _, ok := declarationNameIndexInLine(line, "def", componentName); !ok {
			continue
		}

		openParen := strings.IndexByte(line, '(')
		closeParen := strings.LastIndexByte(line, ')')
		if openParen < 0 || closeParen <= openParen {
			continue
		}

		searchFrom := openParen + 1
		for searchFrom <= closeParen {
			segment := line[searchFrom:closeParen]
			idxInSuffix := findIdentifierInMetaText(segment, portName, 0)
			if idxInSuffix >= 0 {
				idx := searchFrom + idxInSuffix
				return spanRange(lineIdx, idx, idx+len(portName)), true
			}

			prefixIdx := strings.Index(segment, portName)
			if prefixIdx < 0 {
				break
			}

			idx := searchFrom + prefixIdx
			leftBoundary := idx == 0 || !isIdentifierByte(line[idx-1])
			if !leftBoundary {
				searchFrom = idx + 1
				continue
			}

			end := idx + len(portName)
			for end < closeParen && isIdentifierByte(line[end]) {
				end++
			}
			if end > idx {
				return spanRange(lineIdx, idx, end), true
			}
			searchFrom = idx + 1
		}
	}

	return protocol.Range{}, false
}

func interfaceForEntity(entity src.Entity) (src.Interface, bool) {
	switch entity.Kind {
	case src.InterfaceEntity:
		return entity.Interface, true
	case src.ComponentEntity:
		if len(entity.Component) == 0 {
			return src.Interface{}, false
		}
		return entity.Component[0].Interface, true
	default:
		return src.Interface{}, false
	}
}

func (s *Server) findNodeHitAtPosition(ctx *fileContext, pos core.Position) (*nodeHit, bool) {
	var result *nodeHit
	forEachComponent(ctx.file, func(componentName string, component src.Component) bool {
		for nodeName, node := range component.Nodes {
			nodeRange := rangeForName(node.Meta, nodeName, 0)
			if positionInRange(pos, nodeRange) {
				result = &nodeHit{
					componentName: componentName,
					component:     component,
					nodeName:      nodeName,
					node:          node,
					rng:           nodeRange,
				}
				return false
			}
		}

		for _, conn := range component.Net {
			if !forEachConnectionPortAddr(conn, func(addr src.PortAddr, _ bool) bool {
				if isComponentBoundaryNode(addr.Node) {
					return true
				}
				node, ok := component.Nodes[addr.Node]
				if !ok {
					return true
				}
				nodeRange, ok := rangeForNodeInPortAddr(addr)
				if !ok || !positionInRange(pos, nodeRange) {
					return true
				}
				result = &nodeHit{
					componentName: componentName,
					component:     component,
					nodeName:      addr.Node,
					node:          node,
					rng:           nodeRange,
				}
				return false
			}) {
				return false
			}
		}
		return true
	})

	return result, result != nil
}

func (s *Server) resolvePortForAddr(
	build *src.Build,
	ctx *fileContext,
	componentName string,
	component src.Component,
	addr src.PortAddr,
	isSender bool,
	hoverRange protocol.Range,
) (*portHit, bool) {
	if isComponentBoundaryNode(addr.Node) {
		var (
			port      src.Port
			ok        bool
			direction string
		)

		if addr.Node == "in" {
			port, ok = component.Interface.IO.In[addr.Port]
			direction = "in"
		} else {
			port, ok = component.Interface.IO.Out[addr.Port]
			direction = "out"
		}
		if !ok {
			return nil, false
		}
		componentEntity, exists := ctx.file.Entities[componentName]
		entityMeta := componentEntity.Meta()
		fileText := s.fileText(ctx.filePath)
		if expandedRange, expanded := normalizeIdentifierRangeInText(hoverRange, fileText); expanded {
			hoverRange = expandedRange
		}

		currentPortName := addr.Port
		if resolvedPortName, ok := identifierTextInRange(hoverRange, fileText); ok {
			currentPortName = resolvedPortName
		}
		defRange, ok := rangeForPortNameWithFallback(
			port,
			currentPortName,
			ctx.filePath,
			fileText,
			entityMeta,
			componentName,
		)
		if !ok {
			if !exists {
				return nil, false
			}
			if defRange, ok = rangeForEntityDeclaration(componentEntity, componentName, ctx.filePath); !ok {
				return nil, false
			}
		}
		return &portHit{
			name:       currentPortName,
			owner:      componentName,
			direction:  direction,
			typeExpr:   port.TypeExpr.String(),
			defPath:    ctx.filePath,
			defRange:   defRange,
			hoverRange: hoverRange,
		}, true
	}

	node, ok := component.Nodes[addr.Node]
	if !ok {
		return nil, false
	}

	resolved, ok := s.resolveEntityRef(build, ctx, node.EntityRef)
	if !ok {
		return nil, false
	}

	iface, ok := interfaceForEntity(resolved.entity)
	if !ok {
		return nil, false
	}

	primaryPorts := iface.IO.Out
	primaryDirection := "out"
	secondaryPorts := iface.IO.In
	secondaryDirection := "in"
	if !isSender {
		primaryPorts = iface.IO.In
		primaryDirection = "in"
		secondaryPorts = iface.IO.Out
		secondaryDirection = "out"
	}

	port, ok := primaryPorts[addr.Port]
	direction := primaryDirection
	if !ok {
		port, ok = secondaryPorts[addr.Port]
		direction = secondaryDirection
	}
	if !ok {
		return nil, false
	}

	entityMeta := resolved.entity.Meta()
	fileText := s.fileText(resolved.filePath)
	if expandedRange, expanded := normalizeIdentifierRangeInText(hoverRange, fileText); expanded {
		hoverRange = expandedRange
	}
	currentPortName := addr.Port
	if resolvedPortName, ok := identifierTextInRange(hoverRange, fileText); ok {
		currentPortName = resolvedPortName
	}
	defRange, ok := rangeForPortNameWithFallback(
		port,
		currentPortName,
		resolved.filePath,
		fileText,
		entityMeta,
		resolved.name,
	)
	if !ok {
		if defRange, ok = rangeForEntityDeclaration(resolved.entity, resolved.name, resolved.filePath); !ok {
			return nil, false
		}
	}

	return &portHit{
		name:       currentPortName,
		owner:      addr.Node,
		direction:  direction,
		typeExpr:   port.TypeExpr.String(),
		defPath:    resolved.filePath,
		defRange:   defRange,
		hoverRange: hoverRange,
	}, true
}

func (s *Server) findPortHitAtPosition(build *src.Build, ctx *fileContext, pos core.Position) (*portHit, bool) {
	var result *portHit
	fileText := s.fileText(ctx.filePath)
	forEachComponent(ctx.file, func(componentName string, component src.Component) bool {
		componentEntity, hasEntity := ctx.file.Entities[componentName]
		entityMeta := componentEntity.Meta()
		if signatureHit, ok := signaturePortHitAtPosition(fileText, componentName, component.Meta, pos); ok {
			signatureHit.defPath = ctx.filePath
			if signatureHit.direction == "in" {
				if port, exists := component.Interface.IO.In[signatureHit.name]; exists {
					signatureHit.typeExpr = port.TypeExpr.String()
				}
			}
			if signatureHit.direction == "out" {
				if port, exists := component.Interface.IO.Out[signatureHit.name]; exists {
					signatureHit.typeExpr = port.TypeExpr.String()
				}
			}
			result = signatureHit
			return false
		}

		for portName, port := range component.Interface.IO.In {
			defRange, ok := rangeForPortNameWithFallback(port, portName, ctx.filePath, fileText, entityMeta, componentName)
			if ok && positionInRange(pos, defRange) {
				result = &portHit{
					name:       portName,
					owner:      componentName,
					direction:  "in",
					typeExpr:   port.TypeExpr.String(),
					defPath:    ctx.filePath,
					defRange:   defRange,
					hoverRange: defRange,
				}
				return false
			}
		}

		for portName, port := range component.Interface.IO.Out {
			defRange, ok := rangeForPortNameWithFallback(port, portName, ctx.filePath, fileText, entityMeta, componentName)
			if ok && positionInRange(pos, defRange) {
				result = &portHit{
					name:       portName,
					owner:      componentName,
					direction:  "out",
					typeExpr:   port.TypeExpr.String(),
					defPath:    ctx.filePath,
					defRange:   defRange,
					hoverRange: defRange,
				}
				return false
			}
		}

		if !hasEntity {
			return true
		}

		for _, conn := range component.Net {
			if !forEachConnectionPortAddr(conn, func(addr src.PortAddr, isSender bool) bool {
				portRange, ok := rangeForPortInPortAddr(addr)
				if !ok {
					return true
				}
				if expandedRange, expanded := normalizeIdentifierRangeInText(portRange, fileText); expanded {
					portRange = expandedRange
				}
				cursorRange := rangeWithLeadingColon(portRange, fileText)
				if !positionInRange(pos, portRange) && !positionInRange(pos, cursorRange) {
					return true
				}
				hit, ok := s.resolvePortForAddr(build, ctx, componentName, component, addr, isSender, portRange)
				if !ok {
					return true
				}
				result = hit
				return false
			}) {
				return false
			}
		}
		return true
	})

	return result, result != nil
}

// findComponentAtPosition returns the innermost component that contains the cursor.
func findComponentAtPosition(file src.File, pos core.Position) (*componentContext, bool) {
	for name, entity := range file.Entities {
		if entity.Kind != src.ComponentEntity {
			continue
		}
		for _, comp := range entity.Component {
			if metaContains(comp.Meta, pos) {
				return &componentContext{name: name, component: comp}, true
			}
		}
	}
	return nil, false
}

// TextDocumentDefinition returns definition locations for the symbol under cursor.
func (s *Server) TextDocumentDefinition(
	glspCtx *glsp.Context,
	params *protocol.DefinitionParams,
) (any, error) {
	locations := s.definitionLocations(params.TextDocument.URI, params.Position)
	if len(locations) == 0 {
		return []protocol.Location{}, nil
	}
	return locations, nil
}

// definitionLocations resolves definitions for both references and local declarations.
func (s *Server) definitionLocations(
	uri string,
	position protocol.Position,
) []protocol.Location {
	build, ok := s.getBuild()
	if !ok {
		return nil
	}

	ctx, err := s.findFile(build, uri)
	if err != nil {
		if isFileNotFoundInBuild(err) {
			s.logReadOnlyFileMiss("textDocument/definition", uri, err)
		}
		return nil
	}

	pos := lspToCorePosition(position)

	if name, entity, found := s.findEntityDefinitionAtPosition(ctx, pos); found {
		defRange, ok := rangeForEntityDeclaration(*entity, name, ctx.filePath)
		if !ok {
			return nil
		}
		loc := protocol.Location{
			URI:   pathToURI(ctx.filePath),
			Range: defRange,
		}
		return []protocol.Location{loc}
	}

	if node, found := s.findNodeHitAtPosition(ctx, pos); found {
		nodeRange := rangeForName(node.node.Meta, node.nodeName, 0)
		return []protocol.Location{
			{
				URI:   pathToURI(ctx.filePath),
				Range: nodeRange,
			},
		}
	}

	if port, found := s.findPortHitAtPosition(build, ctx, pos); found {
		if port.defPath == "" {
			return nil
		}
		return []protocol.Location{
			{
				URI:   pathToURI(port.defPath),
				Range: port.defRange,
			},
		}
	}

	if impHit, found := findImportAliasAtPosition(ctx, pos); found {
		_, _, _, filePath, ok := s.resolveImportPackage(build, ctx, impHit.alias)
		if ok && filePath != "" {
			return []protocol.Location{
				{
					URI:   pathToURI(filePath),
					Range: protocol.Range{},
				},
			}
		}
	}

	if refHit, found := s.findEntityRefAtPosition(ctx, pos); found {
		if refHit.segment == entityRefSegmentPackage {
			_, _, _, filePath, ok := s.resolveImportPackage(build, ctx, refHit.ref.Pkg)
			if ok && filePath != "" {
				return []protocol.Location{
					{
						URI:   pathToURI(filePath),
						Range: protocol.Range{},
					},
				}
			}
			return nil
		}

		resolved, ok := s.resolveEntityRef(build, ctx, refHit.ref)
		if !ok || resolved.filePath == "" {
			return nil
		}
		defRange, ok := rangeForEntityDeclaration(resolved.entity, resolved.name, resolved.filePath)
		if !ok {
			return nil
		}
		loc := protocol.Location{
			URI:   pathToURI(resolved.filePath),
			Range: defRange,
		}
		return []protocol.Location{loc}
	}

	return nil
}

// TextDocumentImplementation returns definition locations as implementation fallbacks.
func (s *Server) TextDocumentImplementation(
	glspCtx *glsp.Context,
	params *protocol.ImplementationParams,
) (any, error) {
	// Neva currently does not distinguish interface vs implementation at LSP level.
	locations := s.definitionLocations(params.TextDocument.URI, params.Position)
	if len(locations) == 0 {
		return []protocol.Location{}, nil
	}
	return locations, nil
}

// TextDocumentReferences returns all usage locations for the symbol under cursor.
func (s *Server) TextDocumentReferences(
	glspCtx *glsp.Context,
	params *protocol.ReferenceParams,
) ([]protocol.Location, error) {
	build, ok := s.getBuild()
	if !ok {
		return []protocol.Location{}, nil
	}

	ctx, err := s.findFile(build, params.TextDocument.URI)
	if err != nil {
		if isFileNotFoundInBuild(err) {
			s.logReadOnlyFileMiss("textDocument/references", params.TextDocument.URI, err)
			return []protocol.Location{}, nil
		}
		return nil, err
	}

	pos := lspToCorePosition(params.Position)
	var target *resolvedEntity

	if refHit, found := s.findEntityRefAtPosition(ctx, pos); found && refHit.segment == entityRefSegmentName {
		resolved, ok := s.resolveEntityRef(build, ctx, refHit.ref)
		if ok {
			target = resolved
		}
	}
	if target == nil {
		if name, _, found := s.findEntityDefinitionAtPosition(ctx, pos); found {
			resolved, ok := s.resolveEntityRef(build, ctx, core.EntityRef{Name: name})
			if ok {
				target = resolved
			}
		}
	}
	if target == nil {
		return nil, nil
	}

	locations := s.referencesForEntity(build, target)
	if params.Context.IncludeDeclaration {
		locations = s.appendDeclarationLocation(locations, target)
	}

	return locations, nil
}

// renamePort rewrites declaration and network usages for the resolved port target.
func (s *Server) renamePort(build *src.Build, target *portHit, newName string) *protocol.WorkspaceEdit {
	if target == nil || target.name == "" || target.defPath == "" {
		return nil
	}

	edits := map[string][]protocol.TextEdit{}
	appendEdit := func(uri string, edit protocol.TextEdit) {
		edits[uri] = appendUniqueTextEdit(edits[uri], edit)
	}

	s.forEachWorkspaceFile(build, func(fileCtx fileContext) bool {
		fileText := s.fileText(fileCtx.filePath)
		forEachComponent(fileCtx.file, func(componentName string, component src.Component) bool {
			for _, conn := range component.Net {
				forEachConnectionPortAddr(conn, func(addr src.PortAddr, isSender bool) bool {
					portRange, ok := rangeForPortInPortAddr(addr)
					if !ok {
						return true
					}
					if expandedRange, expanded := normalizeIdentifierRangeInText(portRange, fileText); expanded {
						portRange = expandedRange
					}
					hit, ok := s.resolvePortForAddr(build, &fileCtx, componentName, component, addr, isSender, portRange)
					if !ok || !samePortDeclaration(hit, target) {
						return true
					}

					appendEdit(pathToURI(fileCtx.filePath), protocol.TextEdit{
						Range:   portRange,
						NewText: newName,
					})
					return true
				})
			}
			return true
		})
		return true
	})

	appendEdit(pathToURI(target.defPath), protocol.TextEdit{
		Range:   target.defRange,
		NewText: newName,
	})

	if len(edits) == 0 {
		return nil
	}
	return &protocol.WorkspaceEdit{Changes: edits}
}

// TextDocumentPrepareRename validates rename targets and returns editable ranges.
func (s *Server) TextDocumentPrepareRename(
	glspCtx *glsp.Context,
	params *protocol.PrepareRenameParams,
) (any, error) {
	build, ok := s.getBuild()
	if !ok {
		return false, nil
	}
	ctx, err := s.findFile(build, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	pos := lspToCorePosition(params.Position)

	if refHit, found := s.findEntityRefAtPosition(ctx, pos); found && refHit.segment == entityRefSegmentName {
		resolved, ok := s.resolveEntityRef(build, ctx, refHit.ref)
		if !ok {
			return false, nil
		}
		offset := nameOffsetForRef(refHit.meta, refHit.ref)
		r := rangeForName(refHit.meta, resolved.name, offset)
		return r, nil
	}

	if port, found := s.findPortHitAtPosition(build, ctx, pos); found {
		return port.hoverRange, nil
	}

	if name, entity, found := s.findEntityDefinitionAtPosition(ctx, pos); found {
		if r, ok := rangeForEntityDeclaration(*entity, name, ctx.filePath); ok {
			return r, nil
		}
	}

	return false, nil
}

// TextDocumentRename rewrites references and declaration for the target symbol.
//
//nolint:nilnil // LSP allows a nil result when no rename target can be resolved.
func (s *Server) TextDocumentRename(
	glspCtx *glsp.Context,
	params *protocol.RenameParams,
) (*protocol.WorkspaceEdit, error) {
	if !isValidIdentifierName(params.NewName) {
		return nil, fmt.Errorf("invalid identifier name: %q", params.NewName)
	}

	build, ok := s.getBuild()
	if !ok {
		return nil, nil
	}
	ctx, err := s.findFile(build, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}

	pos := lspToCorePosition(params.Position)
	if port, found := s.findPortHitAtPosition(build, ctx, pos); found {
		return s.renamePort(build, port, params.NewName), nil
	}

	var target *resolvedEntity

	if refHit, found := s.findEntityRefAtPosition(ctx, pos); found && refHit.segment == entityRefSegmentName {
		resolved, ok := s.resolveEntityRef(build, ctx, refHit.ref)
		if ok {
			target = resolved
		}
	}
	if target == nil {
		if name, _, found := s.findEntityDefinitionAtPosition(ctx, pos); found {
			resolved, ok := s.resolveEntityRef(build, ctx, core.EntityRef{Name: name})
			if ok {
				target = resolved
			}
		}
	}
	if target == nil {
		return nil, nil
	}

	edits := map[string][]protocol.TextEdit{}

	// Collect edits across all workspace-resolved files.
	s.forEachWorkspaceFile(build, func(fileCtx fileContext) bool {
		refs := collectRefsInFile(fileCtx.file)
		for _, ref := range refs {
			resolved, ok := s.resolveEntityRef(build, &fileCtx, ref.ref)
			if !ok {
				continue
			}
			if resolved.moduleRef.Path != target.moduleRef.Path || resolved.packageName != target.packageName || resolved.name != target.name {
				continue
			}
			offset := nameOffsetForRef(ref.meta, ref.ref)
			r := rangeForName(ref.meta, resolved.name, offset)
			edits[pathToURI(fileCtx.filePath)] = append(edits[pathToURI(fileCtx.filePath)], protocol.TextEdit{
				Range:   r,
				NewText: params.NewName,
			})
		}
		return true
	})

	// Rename definition
	if r, ok := rangeForEntityDeclaration(target.entity, target.name, target.filePath); ok {
		edits[pathToURI(target.filePath)] = append(edits[pathToURI(target.filePath)], protocol.TextEdit{
			Range:   r,
			NewText: params.NewName,
		})
	}

	return &protocol.WorkspaceEdit{Changes: edits}, nil
}

// referencesForEntity collects all locations that resolve to the target entity.
func (s *Server) referencesForEntity(build *src.Build, target *resolvedEntity) []protocol.Location {
	var locations []protocol.Location
	s.forEachWorkspaceFile(build, func(fileCtx fileContext) bool {
		refs := collectRefsInFile(fileCtx.file)
		for _, ref := range refs {
			resolved, ok := s.resolveEntityRef(build, &fileCtx, ref.ref)
			if !ok {
				continue
			}
			if resolved.moduleRef.Path != target.moduleRef.Path || resolved.packageName != target.packageName || resolved.name != target.name {
				continue
			}
			nameOffset := nameOffsetForRef(ref.meta, ref.ref)
			locations = append(locations, protocol.Location{
				URI:   pathToURI(fileCtx.filePath),
				Range: rangeForName(ref.meta, resolved.name, nameOffset),
			})
		}
		return true
	})
	return locations
}

// appendDeclarationLocation appends the declaration location if available.
func (s *Server) appendDeclarationLocation(
	locations []protocol.Location,
	target *resolvedEntity,
) []protocol.Location {
	rng, ok := rangeForEntityDeclaration(target.entity, target.name, target.filePath)
	if !ok {
		return locations
	}
	return append(locations, protocol.Location{
		URI:   pathToURI(target.filePath),
		Range: rng,
	})
}

// TextDocumentHover renders contextual markdown for the symbol under cursor.
//
//nolint:nilnil // LSP allows a nil result when hover content is unavailable.
func (s *Server) TextDocumentHover(
	glspCtx *glsp.Context,
	params *protocol.HoverParams,
) (*protocol.Hover, error) {
	build, ok := s.getBuild()
	if !ok {
		return nil, nil
	}
	ctx, err := s.findFile(build, params.TextDocument.URI)
	if err != nil {
		if isFileNotFoundInBuild(err) {
			s.logReadOnlyFileMiss("textDocument/hover", params.TextDocument.URI, err)
			return nil, nil
		}
		return nil, err
	}

	pos := lspToCorePosition(params.Position)

	if port, found := s.findPortHitAtPosition(build, ctx, pos); found {
		contents := formatPortHover(port)
		return &protocol.Hover{
			Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: contents},
			Range:    &port.hoverRange,
		}, nil
	}

	if node, found := s.findNodeHitAtPosition(ctx, pos); found {
		resolved, ok := s.resolveEntityRef(build, ctx, node.node.EntityRef)
		if ok {
			contents := s.formatNodeHover(node.nodeName, resolved)
			return &protocol.Hover{
				Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: contents},
				Range:    &node.rng,
			}, nil
		}
	}

	if impHit, found := findImportAliasAtPosition(ctx, pos); found {
		modRef, imp, pkg, _, ok := s.resolveImportPackage(build, ctx, impHit.alias)
		if ok {
			contents := s.formatPackageHover(impHit.alias, modRef, imp, pkg)
			return &protocol.Hover{
				Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: contents},
				Range:    &impHit.rng,
			}, nil
		}
	}

	if refHit, found := s.findEntityRefAtPosition(ctx, pos); found {
		if refHit.segment == entityRefSegmentPackage {
			modRef, imp, pkg, _, ok := s.resolveImportPackage(build, ctx, refHit.ref.Pkg)
			if ok {
				rng, _ := entityRefPkgRange(refHit.meta, refHit.ref)
				contents := s.formatPackageHover(refHit.ref.Pkg, modRef, imp, pkg)
				return &protocol.Hover{
					Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: contents},
					Range:    &rng,
				}, nil
			}
			return nil, nil
		}

		resolved, ok := s.resolveEntityRef(build, ctx, refHit.ref)
		if ok {
			r := entityRefNameRange(refHit.meta, refHit.ref)
			contents := s.formatEntityHover(resolved)
			return &protocol.Hover{
				Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: contents},
				Range:    &r,
			}, nil
		}
	}

	if name, entity, found := s.findEntityDefinitionAtPosition(ctx, pos); found {
		resolved, ok := s.resolveEntityRef(build, ctx, core.EntityRef{Name: name})
		if ok {
			r, ok := rangeForEntityDeclaration(*entity, name, ctx.filePath)
			if !ok {
				return nil, nil
			}
			contents := s.formatEntityHover(resolved)
			return &protocol.Hover{
				Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: contents},
				Range:    &r,
			}, nil
		}
	}

	return nil, nil
}

func formatPortHover(port *portHit) string {
	return fmt.Sprintf(
		"```neva\n%s:%s %s // %s port\n```",
		port.owner,
		port.name,
		port.typeExpr,
		port.direction,
	)
}

func (s *Server) formatNodeHover(nodeName string, target *resolvedEntity) string {
	return fmt.Sprintf("Node `%s`\n\n%s", nodeName, s.formatEntityHover(target))
}

func (s *Server) formatPackageHover(alias string, modRef core.ModuleRef, imp src.Import, pkg src.Package) string {
	entities := packageEntities(pkg)
	entityCount := len(entities)
	previewLimit := 6
	if previewLimit > entityCount {
		previewLimit = entityCount
	}

	preview := ""
	if previewLimit > 0 {
		wrapped := make([]string, 0, previewLimit)
		for _, entityRef := range entities[:previewLimit] {
			entityPath := s.pathForLocation(core.Location{
				ModRef:   modRef,
				Package:  imp.Package,
				Filename: entityRef.FileName,
			})

			linkLine := 1
			if declarationRange, ok := rangeForEntityDeclaration(entityRef.Entity, entityRef.EntityName, entityPath); ok {
				linkLine = int(declarationRange.Start.Line) + 1
			}

			if entityPath == "" {
				wrapped = append(wrapped, fmt.Sprintf("`%s`", entityRef.EntityName))
				continue
			}

			entityURI := pathToURI(entityPath)
			wrapped = append(wrapped, fmt.Sprintf("[%s](%s#L%d)", entityRef.EntityName, entityURI, linkLine))
		}
		preview = strings.Join(wrapped, ", ")
	}

	moreSuffix := ""
	if entityCount > previewLimit {
		moreSuffix = fmt.Sprintf(" (+%d more)", entityCount-previewLimit)
	}

	contents := fmt.Sprintf(
		"```neva\nimport %s %s:%s\n```\nModule `%s`",
		alias,
		imp.Module,
		imp.Package,
		modRef.String(),
	)
	if preview != "" {
		contents += fmt.Sprintf("\n\nEntities: %s%s", preview, moreSuffix)
	}
	return contents
}

func packageEntities(pkg src.Package) []src.EntitiesResult {
	entityRefs := []src.EntitiesResult{}
	for entityResult := range pkg.Entities() {
		entityRefs = append(entityRefs, entityResult)
	}
	sort.Slice(entityRefs, func(i int, j int) bool {
		return entityRefs[i].EntityName < entityRefs[j].EntityName
	})
	return entityRefs
}

// formatEntityHover formats a Neva snippet for hover markdown and prepends docs.
func (s *Server) formatEntityHover(target *resolvedEntity) string {
	snippet := formatEntityHoverSnippet(target)
	if declarationRange, ok := rangeForEntityDeclaration(target.entity, target.name, target.filePath); ok {
		if docs := declarationCommentBlock(target.filePath, int(declarationRange.Start.Line)); docs != "" {
			return docs + "\n\n" + snippet
		}
	}
	return snippet
}

func declarationCommentBlock(filePath string, declarationLine int) string {
	if filePath == "" || declarationLine <= 0 {
		return ""
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return ""
	}

	upper := declarationLine - 1
	if upper >= len(lines) {
		upper = len(lines) - 1
	}
	if upper < 0 {
		return ""
	}

	// Directives may be attached between docs and declaration.
	for upper >= 0 {
		trimmed := strings.TrimSpace(lines[upper])
		if trimmed == "" {
			return ""
		}
		if strings.HasPrefix(trimmed, "#") {
			upper--
			continue
		}
		break
	}
	if upper < 0 {
		return ""
	}

	end := upper
	for upper >= 0 {
		trimmed := strings.TrimSpace(lines[upper])
		if !strings.HasPrefix(trimmed, "//") {
			break
		}
		upper--
	}
	start := upper + 1
	if start > end {
		return ""
	}

	docLines := make([]string, 0, end-start+1)
	for i := start; i <= end; i++ {
		line := strings.TrimSpace(lines[i])
		line = strings.TrimPrefix(line, "//")
		line = strings.TrimLeft(line, " \t")
		docLines = append(docLines, line)
	}
	return strings.Join(docLines, "\n")
}

// formatEntityHoverSnippet formats only the declaration snippet for hover markdown.
func formatEntityHoverSnippet(target *resolvedEntity) string {
	switch target.entity.Kind {
	case src.ConstEntity:
		constType := target.entity.Const.TypeExpr.String()
		constValue := target.entity.Const.Value.String()
		return fmt.Sprintf("```neva\nconst %s %s = %s\n```", target.name, constType, constValue)
	case src.TypeEntity:
		if target.entity.Type.BodyExpr == nil {
			return fmt.Sprintf("```neva\ntype %s\n```", target.name)
		}
		return fmt.Sprintf("```neva\ntype %s %s\n```", target.name, target.entity.Type.BodyExpr.String())
	case src.InterfaceEntity:
		return fmt.Sprintf("```neva\ninterface %s%s\n```", target.name, formatInterfaceSignature(target.entity.Interface))
	case src.ComponentEntity:
		return fmt.Sprintf("```neva\ndef %s%s\n```", target.name, formatInterfaceSignature(target.entity.Component[0].Interface))
	default:
		return fmt.Sprintf("```neva\n%s\n```", target.name)
	}
}

// formatInterfaceSignature formats `(in) (out)` interface signatures for hovers.
func formatInterfaceSignature(iface src.Interface) string {
	inParts := make([]string, 0, len(iface.IO.In))
	outParts := make([]string, 0, len(iface.IO.Out))

	inNames := make([]string, 0, len(iface.IO.In))
	for name := range iface.IO.In {
		inNames = append(inNames, name)
	}
	sort.Strings(inNames)
	for _, name := range inNames {
		port := iface.IO.In[name]
		label := name
		if port.IsArray {
			label = "[" + name + "]"
		}
		inParts = append(inParts, fmt.Sprintf("%s %s", label, port.TypeExpr.String()))
	}

	outNames := make([]string, 0, len(iface.IO.Out))
	for name := range iface.IO.Out {
		outNames = append(outNames, name)
	}
	sort.Strings(outNames)
	for _, name := range outNames {
		port := iface.IO.Out[name]
		label := name
		if port.IsArray {
			label = "[" + name + "]"
		}
		outParts = append(outParts, fmt.Sprintf("%s %s", label, port.TypeExpr.String()))
	}

	return fmt.Sprintf("(%s) (%s)", strings.Join(inParts, ", "), strings.Join(outParts, ", "))
}

// TextDocumentDocumentSymbol returns top-level symbols for the current document.
func (s *Server) TextDocumentDocumentSymbol(
	glspCtx *glsp.Context,
	params *protocol.DocumentSymbolParams,
) (any, error) {
	build, ok := s.getBuild()
	if !ok {
		return []protocol.DocumentSymbol{}, nil
	}
	ctx, err := s.findFile(build, params.TextDocument.URI)
	if err != nil {
		if isFileNotFoundInBuild(err) {
			s.logReadOnlyFileMiss("textDocument/documentSymbol", params.TextDocument.URI, err)
			return []protocol.DocumentSymbol{}, nil
		}
		return nil, err
	}

	symbols := make([]protocol.DocumentSymbol, 0, len(ctx.file.Entities))
	for name, entity := range ctx.file.Entities {
		selectionRange, ok := rangeForEntityDeclaration(entity, name, ctx.filePath)
		if !ok {
			continue
		}
		bodyRange := selectionRange
		if meta := entity.Meta(); meta != nil {
			if fullRange, ok := rangeForEntityBody(*meta); ok {
				bodyRange = fullRange
			}
		}
		if !rangeIsValid(bodyRange) || !rangeContains(bodyRange, selectionRange) {
			bodyRange = selectionRange
		}
		symbol := protocol.DocumentSymbol{
			Name:           name,
			Kind:           entitySymbolKind(entity.Kind),
			Range:          bodyRange,
			SelectionRange: selectionRange,
		}
		symbols = append(symbols, symbol)
	}

	return symbols, nil
}

func rangeContains(outer protocol.Range, inner protocol.Range) bool {
	if outer.Start.Line > inner.Start.Line {
		return false
	}
	if outer.Start.Line == inner.Start.Line && outer.Start.Character > inner.Start.Character {
		return false
	}
	if outer.End.Line < inner.End.Line {
		return false
	}
	if outer.End.Line == inner.End.Line && outer.End.Character < inner.End.Character {
		return false
	}
	return true
}

// entitySymbolKind maps Neva entity kinds to LSP document symbol kinds.
func entitySymbolKind(kind src.EntityKind) protocol.SymbolKind {
	switch kind {
	case src.ConstEntity:
		return protocol.SymbolKindConstant
	case src.TypeEntity:
		return protocol.SymbolKindStruct
	case src.InterfaceEntity:
		return protocol.SymbolKindInterface
	case src.ComponentEntity:
		return protocol.SymbolKindFunction
	default:
		return protocol.SymbolKindVariable
	}
}
