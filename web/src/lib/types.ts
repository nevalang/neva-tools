export type Program = {
  modules: ModuleSummary[]
  entryFileIds?: string[]
}

export type ModuleSummary = {
  path: string
  packages: PackageSummary[]
}

export type PackageSummary = {
  name: string
  fileSummaries: FileSummary[]
}

export type FileSummary = {
  id: string
  name: string
  components?: Array<{ id: string; name: string }>
  interfaces?: Array<{ id: string; name: string }>
  types?: Array<{ id: string; name: string }>
  consts?: Array<{ id: string; name: string }>
}

export type SourceAnchor = {
  text?: string
  startLine?: number
  startCol?: number
  endLine?: number
  endCol?: number
}

export type ResolvedRef = {
  canonicalRef: string
  fileId: string
  entityId: string
  entityKind: string
  anchor?: SourceAnchor
}

export type Port = {
  name: string
  type?: string
  order?: number
  array?: boolean
}

export type DINode = {
  id: string
  name: string
  nodeName?: string
  entityRef?: unknown
  resolvedRef?: ResolvedRef
  typeArgs?: string[]
  anchor?: SourceAnchor
  errGuard?: boolean
}

export type NodeItem = {
  id: string
  name: string
  entityRef?: unknown
  resolvedRef?: ResolvedRef
  diArgs?: DINode[]
  anchor?: SourceAnchor
  errGuard?: boolean
}

export type Endpoint = {
  node?: string
  port?: string
  selector?: string[]
  idx?: number | null
  index?: number | null
  kind?: string
  constType?: string
  constValue?: string
}

export type Connection = {
  id: string
  sender: Endpoint
  receiver: Endpoint
  signature?: string
}

export type Component = {
  id: string
  name: string
  inPorts: Port[]
  outPorts: Port[]
  nodes: NodeItem[]
  connections: Connection[]
  anchor?: SourceAnchor
}

export type Interface = {
  id: string
  name: string
  inPorts: Port[]
  outPorts: Port[]
  anchor?: SourceAnchor
}

export type TypeDecl = {
  id: string
  name: string
  type?: string
  anchor?: SourceAnchor
}

export type ConstDecl = {
  id: string
  name: string
  type?: string
  value?: string
  anchor?: SourceAnchor
}

export type ImportRef = {
  id?: string
  alias?: string
  module?: string
  package?: string
  path?: string
}

export type FileView = {
  id: string
  name: string
  components: Component[]
  interfaces: Interface[]
  types: TypeDecl[]
  consts: ConstDecl[]
  imports: ImportRef[]
}

export type ManifestView = {
  path: string
  raw: string
  deps: Record<string, string>
  present: boolean
}

export type ResolveEntityRefResult = {
  targetKind: string
  targetName: string
  targetFileId: string
  targetEntityId: string
  targetAnchor?: SourceAnchor
}

export type SearchEntitiesResultItem = {
  label: string
  kind: 'component' | 'interface' | 'type' | 'const' | string
  module: string
  package: string
  fileId: string
  entityId: string
  anchor?: SourceAnchor
}
