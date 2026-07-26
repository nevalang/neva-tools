import type {
  Component,
  Connection,
  DINode,
  Endpoint,
  FileView,
  Interface,
  ModuleSummary,
  NodeItem,
  PackageSummary,
  Port,
  Program,
  ResolveEntityRefResult,
  SearchEntitiesResultItem,
} from './types'

export function normalizeProgram(program: Program): Program {
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
    typeArgs: node.typeArgs ?? [],
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

export interface ViewBackend {
  getProgram(filters: ProgramFilters): Promise<Program>
  getFileView(fileId: string): Promise<FileView>
  searchEntities(filters: SearchFilters): Promise<SearchEntitiesResultItem[]>
  resolveEntityRef(targetFileId: string, targetEntityId: string): Promise<ResolveEntityRefResult>
  onRefresh(listener: () => void): () => void
}
