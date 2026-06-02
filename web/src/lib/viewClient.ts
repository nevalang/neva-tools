import type {
  Component,
  Connection,
  DINode,
  Endpoint,
  FileView,
  Interface,
  ManifestView,
  ModuleSummary,
  NodeItem,
  PackageSummary,
  Port,
  Program,
  ResolveEntityRefResult,
  SearchEntitiesResultItem,
} from './types'

async function getJSON<T>(url: string): Promise<T> {
  const response = await fetch(url)
  if (!response.ok) {
    throw new Error(await response.text())
  }
  return (await response.json()) as T
}

async function postJSON<TReq extends object, TRes>(url: string, payload: TReq): Promise<TRes> {
  const response = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!response.ok) {
    throw new Error(await response.text())
  }
  return (await response.json()) as TRes
}

function normalizeProgram(program: Program): Program {
  return {
    ...program,
    entryFileIds: program.entryFileIds ?? [],
    modules: (program.modules ?? []).map(normalizeModule),
  }
}

function normalizeModule(moduleItem: ModuleSummary): ModuleSummary {
  return {
    ...moduleItem,
    packages: (moduleItem.packages ?? []).map(normalizePackage),
  }
}

function normalizePackage(pkg: PackageSummary): PackageSummary {
  return {
    ...pkg,
    fileSummaries: pkg.fileSummaries ?? [],
  }
}

function normalizePort(port: Partial<Port>): Port {
  return {
    name: port.name ?? '',
    type: port.type,
    order: typeof port.order === 'number' ? port.order : undefined,
    array: Boolean(port.array),
  }
}

function normalizeDINode(node: Partial<DINode>): DINode {
  return {
    id: node.id ?? '',
    name: node.name ?? '',
    nodeName: node.nodeName,
    entityRef: node.entityRef,
    resolvedRef: node.resolvedRef,
    typeArgs: node.typeArgs ?? [],
    anchor: node.anchor,
    errGuard: Boolean(node.errGuard),
  }
}

function normalizeNode(node: Partial<NodeItem>): NodeItem {
  return {
    id: node.id ?? '',
    name: node.name ?? '',
    entityRef: node.entityRef,
    resolvedRef: node.resolvedRef,
    diArgs: (node.diArgs ?? []).map((item) => normalizeDINode(item)),
    anchor: node.anchor,
    errGuard: Boolean((node as any).errGuard),
  }
}

function normalizeEndpoint(endpoint: Partial<Endpoint> | undefined): Endpoint {
  return {
    node: endpoint?.node,
    port: endpoint?.port,
    selector: Array.isArray((endpoint as any)?.selector) ? (endpoint as any).selector : [],
    idx: endpoint?.idx ?? (endpoint as any)?.index ?? null,
    kind: endpoint?.kind,
    constType: endpoint?.constType,
    constValue: endpoint?.constValue,
  }
}

function normalizeConnections(raw: any): Connection[] {
  if (raw && Array.isArray(raw.senders) && Array.isArray(raw.receivers) && raw.senders.length > 0 && raw.receivers.length > 0) {
    const expanded: Connection[] = []
    for (let si = 0; si < raw.senders.length; si++) {
      for (let ri = 0; ri < raw.receivers.length; ri++) {
        expanded.push({
          id: `${raw.id ?? ''}#${si}:${ri}`,
          sender: normalizeEndpoint(raw.senders[si]),
          receiver: normalizeEndpoint(raw.receivers[ri]),
          signature: raw.signature,
        })
      }
    }
    return expanded
  }

  return [{
    id: raw?.id ?? '',
    sender: normalizeEndpoint(raw?.sender),
    receiver: normalizeEndpoint(raw?.receiver),
    signature: raw?.signature,
  }]
}

function normalizeComponent(raw: any): Component {
  return {
    id: raw?.id ?? '',
    name: raw?.name ?? '',
    inPorts: (raw?.inPorts ?? raw?.inports ?? []).map((port: Port) => normalizePort(port)),
    outPorts: (raw?.outPorts ?? raw?.outports ?? []).map((port: Port) => normalizePort(port)),
    nodes: (raw?.nodes ?? []).map((node: NodeItem) => normalizeNode(node)),
    connections: (raw?.connections ?? []).flatMap((connection: any) => normalizeConnections(connection)),
    anchor: raw?.anchor,
  }
}

function normalizeInterface(raw: any): Interface {
  return {
    id: raw?.id ?? '',
    name: raw?.name ?? '',
    inPorts: (raw?.inPorts ?? raw?.inports ?? []).map((port: Port) => normalizePort(port)),
    outPorts: (raw?.outPorts ?? raw?.outports ?? []).map((port: Port) => normalizePort(port)),
    anchor: raw?.anchor,
  }
}

export function normalizeFile(file: any): FileView {
  return {
    id: file?.id ?? '',
    name: file?.name ?? '',
    components: (file?.components ?? []).map((component: any) => normalizeComponent(component)),
    interfaces: (file?.interfaces ?? []).map((iface: any) => normalizeInterface(iface)),
    types: file?.types ?? [],
    consts: file?.consts ?? [],
    imports: file?.imports ?? [],
  }
}

function normalizeManifest(manifest: ManifestView): ManifestView {
  return {
    ...manifest,
    deps: manifest.deps ?? {},
    raw: manifest.raw ?? '',
    path: manifest.path ?? '',
  }
}

export type ProgramFilters = {
  includeCurrent: boolean
  includeDeps: boolean
  includeStd: boolean
}

export type SearchFilters = {
  query: string
  kinds: string[]
  packages: string[]
  modules: string[]
}

export const viewClient = {
  async getProgram(filters: ProgramFilters): Promise<Program> {
    const query = new URLSearchParams({
      includeCurrent: String(filters.includeCurrent),
      includeDeps: String(filters.includeDeps),
      includeStd: String(filters.includeStd),
    })
    return normalizeProgram(await getJSON<Program>(`/api/view/program?${query.toString()}`))
  },

  async getFileView(fileId: string): Promise<FileView> {
    return normalizeFile(await getJSON<any>(`/api/view/file?id=${encodeURIComponent(fileId)}`))
  },

  searchEntities(filters: SearchFilters): Promise<SearchEntitiesResultItem[]> {
    const query = new URLSearchParams({ q: filters.query.trim() })
    for (const kind of filters.kinds) query.append('kind', kind)
    for (const pkg of filters.packages) query.append('package', pkg)
    for (const modulePath of filters.modules) query.append('module', modulePath)
    return getJSON<SearchEntitiesResultItem[]>(`/api/view/search?${query.toString()}`)
  },

  resolveEntityRef(targetFileId: string, targetEntityId: string): Promise<ResolveEntityRefResult> {
    return postJSON('/api/view/resolve', { targetFileId, targetEntityId })
  },

  async getManifest(modulePath: string): Promise<ManifestView> {
    return normalizeManifest(await getJSON<ManifestView>(`/api/view/manifest?module=${encodeURIComponent(modulePath)}`))
  },
}
