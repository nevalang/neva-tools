import { useEffect, useState } from 'react'
import type { MouseEvent } from 'react'
import {
  applyNodeChanges,
  Background,
  ControlButton,
  Controls,
  Handle,
  MiniMap,
  Position,
  ReactFlow,
  type ReactFlowInstance,
  type Edge,
  type Node,
  type NodeChange,
  type NodeProps,
} from '@xyflow/react'
import ELK from 'elkjs/lib/elk.bundled.js'
import { endpointPortName, inferImplicitPortName, parseSignaturePorts, shouldAddImplicitErrEdge, shouldAddImplicitInputEdge } from '../lib/graphSemantics'
import { isNativeComponent, type AppRoute } from '../lib/appSemantics'
import type { Component, ConstDecl, DINode, Endpoint, FileView, ModuleSummary, Port, ResolvedRef, TypeDecl } from '../lib/types'
import { routeToHash } from '../lib/routeCodec'

type Route = AppRoute

type Breadcrumb = {
  key: string
  label: string
  route: Route
}

type Props = {
  modules: ModuleSummary[]
  route: Route
  file: FileView | null
  breadcrumbs: Breadcrumb[]
  canGoBack: boolean
  canGoForward: boolean
  onGoBack: () => void
  onGoForward: () => void
  onNavigate: (route: Route, trackNav?: boolean) => void
  onResolveOpen: (target: { fileId: string; entityId: string }) => Promise<void>
}

export type NodeData = {
  kind: 'entity' | 'port' | 'nav' | 'const'
  navType?: 'module' | 'package' | 'file' | 'component' | 'interface' | 'type' | 'const'
  portRole?: 'in' | 'out'
  label: string
  subtitle?: string
  detail?: string
  showMeta?: boolean
  inPorts?: Port[]
  outPorts?: Port[]
  diArgs?: DINode[]
  onOpenEntity?: (target: { fileId: string; entityId: string }) => void
  fileId?: string
  entityId?: string
  modulePath?: string
  packageName?: string
}

const elk = new ELK()
const THEME_STORAGE_KEY = 'neva-lsp:theme'
const SNAP_GRID_STORAGE_KEY = 'neva-lsp:snap-grid-level'
const SNAP_GRID_STEPS = [0, 12, 24] as const

const LIGHT_KIND_COLORS = {
  module: '#5f8fc8',
  package: '#d28b46',
  file: '#6fa98b',
  component: '#c99646',
  interface: '#5c86c8',
  type: '#4fa693',
  const: '#c96e56',
} as const

const DARK_KIND_COLORS = {
  module: '#7daddb',
  package: '#e0a55f',
  file: '#86c0a1',
  component: '#dfad65',
  interface: '#78a3df',
  type: '#64c1ab',
  const: '#d98269',
} as const

function handleOffsets(count: number): string[] {
  if (count <= 0) {
    return []
  }
  const offsets: string[] = []
  for (let i = 0; i < count; i++) {
    offsets.push(`${((i + 0.5) * 100) / count}%`)
  }
  return offsets
}

function handleIDForPort(portName: string): string {
  return `port:${portName}`
}

function entityKindLabel(navType: NodeData['navType'], subtitle?: string): string | undefined {
  if (navType === 'component') return subtitle
  if (navType === 'interface') return 'interface'
  if (navType === 'type') return subtitle ? `type · ${subtitle}` : 'type'
  if (navType === 'const') return subtitle ? `const · ${subtitle}` : 'const'
  return subtitle
}

function snapGridLabel(level: number): string {
  const size = SNAP_GRID_STEPS[level] ?? 0
  return size <= 0 ? 'Off' : `${size}px`
}

function EntityNode({ data }: NodeProps<Node<NodeData>>) {
  if (data.kind === 'const') {
    return (
      <div className="rf-const-node">
        <Handle type="target" position={Position.Top} style={{ left: '50%' }} />
        <div className="rf-node-title">{data.label}</div>
        {data.subtitle ? <div className="rf-node-subtitle">{data.subtitle}</div> : null}
        <Handle type="source" position={Position.Bottom} style={{ left: '50%' }} />
      </div>
    )
  }

  if (data.kind === 'port') {
    return (
      <div className="rf-port-node">
        {data.portRole === 'out' ? <Handle type="target" position={Position.Top} style={{ left: '50%' }} /> : null}
        <div className="rf-port-name">{data.label}</div>
        {data.showMeta && data.subtitle ? <div className="rf-port-type">{data.subtitle}</div> : null}
        {data.portRole === 'in' ? <Handle type="source" position={Position.Bottom} style={{ left: '50%' }} /> : null}
      </div>
    )
  }

  const inHandles = handleOffsets(data.inPorts?.length ?? 0)
  const outHandles = handleOffsets(data.outPorts?.length ?? 0)
  const showPortBars = Boolean(data.showMeta)
  const hasInPorts = (data.inPorts?.length ?? 0) > 0
  const hasOutPorts = (data.outPorts?.length ?? 0) > 0
  const showConnectionHandles = data.kind === 'entity'
  const inputHandleStyle = { opacity: 1, pointerEvents: 'none' as const, zIndex: 2, top: showPortBars ? 0 : undefined }
  const outputHandleStyle = { opacity: 1, pointerEvents: 'none' as const, zIndex: 2, bottom: showPortBars ? 0 : undefined }
  const diArgs = data.diArgs ?? []

  function openDIArg(event: MouseEvent<HTMLButtonElement>, diArg: DINode) {
    event.stopPropagation()
    const ref = diArg.resolvedRef
    if (!ref?.fileId || !ref.entityId) {
      return
    }
    data.onOpenEntity?.({ fileId: ref.fileId, entityId: ref.entityId })
  }

  return (
    <div className={`rf-node${showPortBars && hasInPorts ? ' rf-node-has-inbars' : ''}${showPortBars && hasOutPorts ? ' rf-node-has-outbars' : ''}`}>
      <div className="rf-node-frame">
        {showPortBars && hasInPorts && (
          <div className="rf-node-port-row rf-node-port-row-top">
            {data.inPorts?.map((port, idx) => (
              <span
                key={`in-pill-${port.name}`}
                className="rf-port-pill rf-port-pill-top"
                style={{ left: inHandles[idx], width: `${100 / (data.inPorts?.length || 1)}%` }}
              >
                <span className="rf-port-name">{port.name}</span>
                {port.type ? <span className="rf-port-type">{port.type}</span> : null}
              </span>
            ))}
          </div>
        )}
        <div className="rf-node-body">
          <div className="rf-node-title">{data.label}</div>
          {data.showMeta && data.subtitle && <div className="rf-node-subtitle">{data.subtitle}</div>}
          {data.showMeta && !data.subtitle && data.navType && entityKindLabel(data.navType) && (
            <div className="rf-node-subtitle">{entityKindLabel(data.navType)}</div>
          )}
          {data.detail ? <div className="rf-node-detail">{data.detail}</div> : null}
          {diArgs.length > 0 ? (
            <div className="rf-di-list" aria-label="Dependency injections">
              {diArgs.map((diArg) => {
                const clickable = Boolean(diArg.resolvedRef?.fileId && diArg.resolvedRef?.entityId)
                const label = diDisplayName(diArg)
                return (
                  <button
                    key={diArg.id || `${diArg.name}:${label}`}
                    type="button"
                    className={`rf-di-node${clickable ? ' rf-di-node-clickable' : ''}`}
                    onClick={(event) => openDIArg(event, diArg)}
                    disabled={!clickable}
                    title={clickable ? `Open ${label}` : label}
                  >
                    {diArg.name ? <span className="rf-di-slot">{diArg.name}:</span> : null}
                    <span className="rf-di-target">{label}</span>
                  </button>
                )
              })}
            </div>
          ) : null}
        </div>
        {showPortBars && hasOutPorts && (
          <div className="rf-node-port-row rf-node-port-row-bottom">
            {data.outPorts?.map((port, idx) => (
              <span
                key={`out-pill-${port.name}`}
                className="rf-port-pill rf-port-pill-bottom"
                style={{ left: outHandles[idx], width: `${100 / (data.outPorts?.length || 1)}%` }}
              >
                <span className="rf-port-name">{port.name}</span>
                {data.showMeta && port.type ? <span className="rf-port-type">{port.type}</span> : null}
              </span>
            ))}
          </div>
        )}
      </div>
      {showConnectionHandles && inHandles.map((left, idx) => (
        <Handle
          key={`en-in-${idx}`}
          id={handleIDForPort(data.inPorts?.[idx]?.name ?? `in-${idx}`)}
          type="target"
          position={Position.Top}
          style={showPortBars ? { ...inputHandleStyle, left } : { left }}
        />
      ))}
      {showConnectionHandles && outHandles.map((left, idx) => (
        <Handle
          key={`en-out-${idx}`}
          id={handleIDForPort(data.outPorts?.[idx]?.name ?? `out-${idx}`)}
          type="source"
          position={Position.Bottom}
          style={showPortBars ? { ...outputHandleStyle, left } : { left }}
        />
      ))}
    </div>
  )
}

const nodeTypes = { entityNode: EntityNode }

function entityRefName(entityRef: unknown): string {
  if (!entityRef || typeof entityRef !== 'object') {
    return ''
  }
  const raw = entityRef as Record<string, unknown>
  const pkg = typeof raw.pkg === 'string' ? raw.pkg : ''
  const name = typeof raw.name === 'string' ? raw.name : ''
  return pkg && name ? `${pkg}.${name}` : name
}

function resolvedRefName(ref: ResolvedRef | undefined): string {
  const text = ref?.anchor?.text?.trim()
  if (text) {
    const match = text.match(/^([A-Za-z_][A-Za-z0-9_]*)/)
    if (match) return match[1]
  }
  const canonical = ref?.canonicalRef
  if (!canonical) {
    return ''
  }
  return canonical.split('/').at(-1) ?? canonical
}

function diDisplayName(diArg: DINode): string {
  return entityRefName(diArg.entityRef) || resolvedRefName(diArg.resolvedRef) || diArg.nodeName || diArg.name || '?'
}

function endpointNodeID(componentID: string, localNodeName: string, portName?: string): string {
  if (localNodeName === 'in') {
    return `${componentID}::in::${portName ?? '_'}`
  }
  if (localNodeName === 'out') {
    return `${componentID}::out::${portName ?? '_'}`
  }
  return `${componentID}::node::${localNodeName}`
}

function constNodeID(componentID: string, endpoint: Endpoint): string {
  return `${componentID}::const::${endpoint.constType ?? ''}::${endpoint.constValue ?? ''}`
}

function minimapNodeFill(node: Node<NodeData>, theme: 'light' | 'dark'): string {
  const navType = node.data?.navType
  const palette = theme === 'dark' ? DARK_KIND_COLORS : LIGHT_KIND_COLORS
  if (navType === 'module') return palette.module
  if (navType === 'package') return palette.package
  if (navType === 'file') return palette.file
  if (navType === 'component') return palette.component
  if (navType === 'interface') return palette.interface
  if (navType === 'type') return palette.type
  if (navType === 'const') return palette.const
  return theme === 'dark' ? '#9ca6b5' : '#566276'
}

function selectorLabel(endpoint: Endpoint): string {
  if (!endpoint.selector || endpoint.selector.length === 0) {
    return ''
  }
  return endpoint.selector.map((part) => `.${part}`).join('')
}

function resolveNodeID(component: Component, endpoint: Endpoint): string | null {
  if (endpoint.kind === 'const') {
    return constNodeID(component.id, endpoint)
  }
  if (!endpoint.node) {
    return null
  }
  if (endpoint.node === 'in') {
    const port = endpoint.port || component.inPorts[0]?.name
    return endpointNodeID(component.id, 'in', port)
  }
  if (endpoint.node === 'out') {
    const port = endpoint.port || component.outPorts[0]?.name
    return endpointNodeID(component.id, 'out', port)
  }
  return endpointNodeID(component.id, endpoint.node)
}

function inferSelectorSourceInPort(component: Component): string | null {
  if (component.inPorts.length === 0) {
    return null
  }
  const explicitlyUsed = new Set<string>()
  for (const connection of component.connections) {
    if ((connection.sender?.node ?? '') !== 'in') {
      continue
    }
    const port = (connection.sender?.port ?? '').trim()
    if (port) explicitlyUsed.add(port)
  }
  const candidates = component.inPorts.map((port) => port.name).filter((name) => !explicitlyUsed.has(name))
  if (candidates.length === 1) {
    return candidates[0]
  }
  return component.inPorts[0]?.name ?? null
}

function moduleNodes(modules: ModuleSummary[]): Node<NodeData>[] {
  return [...modules].sort((a, b) => a.path.localeCompare(b.path)).map((mod) => ({
    id: `module:${mod.path}`,
    type: 'entityNode',
    className: 'rf-node-clickable rf-node-kind-module',
    position: { x: 0, y: 0 },
    data: {
      kind: 'nav',
      navType: 'module',
      label: mod.path,
      subtitle: `${mod.packages.length} packages`,
      showMeta: true,
      modulePath: mod.path,
    },
  }))
}

function packageNodes(modules: ModuleSummary[], modulePath: string): Node<NodeData>[] {
  const moduleItem = modules.find((item) => item.path === modulePath)
  if (!moduleItem) return []
  return [...moduleItem.packages].sort((a, b) => a.name.localeCompare(b.name)).map((pkg) => ({
    id: `package:${modulePath}:${pkg.name}`,
    type: 'entityNode',
    className: 'rf-node-clickable rf-node-kind-package',
    position: { x: 0, y: 0 },
    data: {
      kind: 'nav',
      navType: 'package',
      label: pkg.name,
      subtitle: `${pkg.fileSummaries.length} files`,
      showMeta: true,
      modulePath,
      packageName: pkg.name,
    },
  }))
}

function fileNodes(modules: ModuleSummary[], modulePath: string, packageName: string): Node<NodeData>[] {
  const moduleItem = modules.find((item) => item.path === modulePath)
  const pkg = moduleItem?.packages.find((item) => item.name === packageName)
  if (!pkg) return []
  return [...pkg.fileSummaries].sort((a, b) => a.name.localeCompare(b.name)).map((file) => ({
    id: `file:${file.id}`,
    type: 'entityNode',
    className: 'rf-node-clickable rf-node-kind-file',
    position: { x: 0, y: 0 },
    data: {
      kind: 'nav',
      navType: 'file',
      label: `${file.name}.neva`,
      subtitle: `${modulePath}/${packageName}`,
      showMeta: true,
      fileId: file.id,
    },
  }))
}

function canDrillComponent(component: Component): boolean {
  return component.nodes.length > 0 || component.connections.length > 0
}

function typePreview(item: TypeDecl): string {
  const raw = item.type?.trim()
  if (!raw) return 'type'
  const normalized = raw.replace(/\s+/g, ' ')
  if (/=\s*\{/u.test(normalized)) {
    return 'struct'
  }
  const match = normalized.match(/^(struct|list|dict|stream|error|any|bool|string|int|float)\b(?:<(.*?)>)?/u)
  if (!match) return 'type'
  const kind = match[1] ?? 'type'
  const args = match[2] ? `<${match[2]}>` : ''
  return `${kind}${args}`
}

function constTypePreview(item: ConstDecl): string {
  return item.type?.trim() || item.anchor?.text?.trim() || 'const'
}

function constValuePreview(item: ConstDecl): string {
  return item.value?.trim() || ''
}

function orderedPortList(portMap: Map<string, string> | undefined, parsedPorts: Map<string, string>): Port[] {
  if (!portMap) {
    return []
  }
  const seen = new Set<string>()
  const result: Port[] = []

  for (const [name, parsedType] of parsedPorts.entries()) {
    if (!portMap.has(name)) {
      continue
    }
    seen.add(name)
    result.push({ name, type: portMap.get(name) || parsedType })
  }

  for (const [name, type] of portMap.entries()) {
    if (seen.has(name)) {
      continue
    }
    result.push({ name, type })
  }

  return result
}

function estimatedPortPillWidth(port: Port): number {
  const nameWidth = (port.name?.length ?? 0) * 8
  const typeWidth = (port.type?.length ?? 0) * 7
  const gap = port.type ? 10 : 0
  return Math.max(92, nameWidth + typeWidth + gap + 24)
}

function numericNodeWidth(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value
  }
  if (typeof value === 'string') {
    const parsed = Number.parseFloat(value)
    if (Number.isFinite(parsed)) {
      return parsed
    }
  }
  return null
}

function estimatedWrappedLines(text: string | undefined, width: number): number {
  if (!text) return 0
  const charsPerLine = Math.max(18, Math.floor((width - 28) / 7))
  return Math.max(1, Math.ceil(text.length / charsPerLine))
}

function fileEntityNodeWidth(node: Node<NodeData>): number {
  const navType = node.data.navType
  if (navType === 'const') {
    return 380
  }
  if (navType === 'component') {
    return 320
  }
  return 300
}

function fileEntityNodeHeight(node: Node<NodeData>): number {
  const width = fileEntityNodeWidth(node)
  const detailLines = estimatedWrappedLines(node.data.detail, width)
  const subtitleLines = node.data.subtitle ? 1 : 0
  return 72 + subtitleLines * 18 + detailLines * 18
}

function fallbackNodeWidth(node: Node<NodeData>): number {
  const styledWidth = numericNodeWidth(node.style?.width)
  if (styledWidth !== null) {
    return Math.max(240, styledWidth)
  }
  if (node.data.kind === 'port') return 88
  if (node.data.kind === 'const') return 92
  if (node.data.kind === 'nav') return fileEntityNodeWidth(node)

  const inWidth = (node.data.inPorts ?? []).reduce((sum, port) => sum + estimatedPortPillWidth(port), 0)
  const outWidth = (node.data.outPorts ?? []).reduce((sum, port) => sum + estimatedPortPillWidth(port), 0)
  return Math.max(240, inWidth, outWidth)
}

function fallbackNodeHeight(node: Node<NodeData>): number {
  if (node.data.kind === 'port') return 70
  if (node.data.kind === 'const') return 72
  if (node.data.kind === 'nav') return fileEntityNodeHeight(node)
  return 120 + (node.data.diArgs?.length ?? 0) * 30
}

function nodeSize(node: Node<NodeData>, measuredSizes?: Map<string, { width: number; height: number }>): { width: number; height: number } {
  return measuredSizes?.get(node.id) ?? { width: fallbackNodeWidth(node), height: fallbackNodeHeight(node) }
}

export function fileEntityNodes(file: FileView, selectedEntityID?: string): Node<NodeData>[] {
  const components = file.components.map((component) => ({
    id: `entity:${component.id}`,
    type: 'entityNode' as const,
    className: `${canDrillComponent(component) ? 'rf-node-clickable ' : ''}rf-node-kind-component${component.id === selectedEntityID ? ' selected' : ''}`,
    position: { x: 0, y: 0 },
    style: { width: 320 },
    data: {
      kind: 'nav' as const,
      navType: 'component' as const,
      label: component.name,
      subtitle: canDrillComponent(component) ? 'component' : 'component (native)',
      showMeta: true,
      fileId: file.id,
      entityId: component.id,
      inPorts: component.inPorts,
      outPorts: component.outPorts,
    },
  }))

  const interfaces = file.interfaces.map((iface) => ({
    id: `entity:${iface.id}`,
    type: 'entityNode' as const,
    className: `rf-node-clickable rf-node-kind-interface${iface.id === selectedEntityID ? ' selected' : ''}`,
    position: { x: 0, y: 0 },
    style: { width: 300 },
    data: {
      kind: 'nav' as const,
      navType: 'interface' as const,
      label: iface.name,
      subtitle: 'interface',
      showMeta: true,
      fileId: file.id,
      entityId: iface.id,
      inPorts: iface.inPorts,
      outPorts: iface.outPorts,
    },
  }))

  const types = file.types.map((item) => ({
    id: `entity:${item.id}`,
    type: 'entityNode' as const,
    className: `rf-node-clickable rf-node-kind-type${item.id === selectedEntityID ? ' selected' : ''}`,
    position: { x: 0, y: 0 },
    style: { width: 300 },
    data: {
      kind: 'nav' as const,
      navType: 'type' as const,
      label: item.name,
      subtitle: `type · ${typePreview(item)}`,
      showMeta: true,
      fileId: file.id,
      entityId: item.id,
    },
  }))

  const consts = file.consts.map((item) => ({
    id: `entity:${item.id}`,
    type: 'entityNode' as const,
    className: `rf-node-clickable rf-node-kind-const${item.id === selectedEntityID ? ' selected' : ''}`,
    position: { x: 0, y: 0 },
    style: { width: 380 },
    data: {
      kind: 'nav' as const,
      navType: 'const' as const,
      label: item.name,
      subtitle: `const · ${constTypePreview(item)}`,
      detail: constValuePreview(item),
      showMeta: true,
      fileId: file.id,
      entityId: item.id,
    },
  }))

  return [...components, ...interfaces, ...types, ...consts]
}

function componentDetailNodes(
  component: Component,
  showMeta: boolean,
  onOpenEntity?: (target: { fileId: string; entityId: string }) => void,
): Node<NodeData>[] {
  const result: Node<NodeData>[] = []
  const nodePortKinds = new Map<string, { in: Map<string, string>; out: Map<string, string> }>()
  const constNodes = new Map<string, Endpoint>()

  for (const connection of component.connections) {
    if (connection.sender?.kind === 'const') {
      constNodes.set(constNodeID(component.id, connection.sender), connection.sender)
    }
    if (connection.receiver?.kind === 'const') {
      constNodes.set(constNodeID(component.id, connection.receiver), connection.receiver)
    }
    if (connection.sender?.node && connection.sender.node !== 'in' && connection.sender.node !== 'out') {
      const item = nodePortKinds.get(connection.sender.node) ?? { in: new Map<string, string>(), out: new Map<string, string>() }
      const portName = endpointPortName(component, connection.sender, 'out')
      item.out.set(portName, '')
      nodePortKinds.set(connection.sender.node, item)
    }
    if (connection.receiver?.node && connection.receiver.node !== 'in' && connection.receiver.node !== 'out') {
      const item = nodePortKinds.get(connection.receiver.node) ?? { in: new Map<string, string>(), out: new Map<string, string>() }
      const portName = endpointPortName(component, connection.receiver, 'in')
      item.in.set(portName, '')
      nodePortKinds.set(connection.receiver.node, item)
    }
  }

  for (const node of component.nodes) {
    const inferredInPort = inferImplicitPortName(component, node.name, 'in')
    if (shouldAddImplicitInputEdge(component, node.name, inferredInPort)) {
      const item = nodePortKinds.get(node.name) ?? { in: new Map<string, string>(), out: new Map<string, string>() }
      const componentInType = component.inPorts.find((port) => port.name === inferredInPort)?.type ?? ''
      item.in.set(inferredInPort, componentInType)
      nodePortKinds.set(node.name, item)
    }

    if (!shouldAddImplicitErrEdge(component, node.name)) {
      continue
    }
    const item = nodePortKinds.get(node.name) ?? { in: new Map<string, string>(), out: new Map<string, string>() }
    item.out.set('err', 'error')
    nodePortKinds.set(node.name, item)
  }

  for (const node of component.nodes) {
    const ports = nodePortKinds.get(node.name)
    if (!ports) continue
    const parsed = parseSignaturePorts(
      node.resolvedRef?.anchor?.text,
      Array.from(ports.in.keys()),
      Array.from(ports.out.keys()),
    )
    for (const [name, t] of parsed.in.entries()) {
      if (ports.in.has(name)) ports.in.set(name, t)
    }
    for (const [name, t] of parsed.out.entries()) {
      if (ports.out.has(name)) ports.out.set(name, t)
    }
  }

  for (const [id, endpoint] of constNodes.entries()) {
    result.push({
      id,
      type: 'entityNode',
      className: 'rf-node-kind-literal',
      position: { x: 0, y: 0 },
      data: {
        kind: 'const',
        label: endpoint.constValue ?? '?',
        subtitle: endpoint.constType,
        showMeta,
      },
    })
  }

  for (const port of component.inPorts) {
    result.push({
      id: endpointNodeID(component.id, 'in', port.name),
      type: 'entityNode',
      position: { x: 0, y: 0 },
      data: {
        kind: 'port',
        portRole: 'in',
        label: port.name,
        subtitle: port.type,
        showMeta,
      },
    })
  }

  for (const node of component.nodes) {
    const ref = node.resolvedRef
    const sourceLikeRef = entityRefName(node.entityRef) || ref?.canonicalRef
    const ports = nodePortKinds.get(node.name)
    const parsed = parseSignaturePorts(
      node.resolvedRef?.anchor?.text,
      Array.from(ports?.in.keys() ?? []),
      Array.from(ports?.out.keys() ?? []),
    )
    result.push({
      id: endpointNodeID(component.id, node.name),
      type: 'entityNode',
      className: node.resolvedRef?.fileId && node.resolvedRef?.entityId ? 'rf-node-clickable rf-node-kind-call' : 'rf-node-kind-call',
      position: { x: 0, y: 0 },
      data: {
        kind: 'entity',
        label: node.name,
        subtitle: sourceLikeRef,
        showMeta,
        inPorts: orderedPortList(ports?.in, parsed.in),
        outPorts: orderedPortList(ports?.out, parsed.out),
        diArgs: node.diArgs,
        onOpenEntity,
        fileId: ref?.fileId,
        entityId: ref?.entityId,
      },
    })
  }

  for (const port of component.outPorts) {
    result.push({
      id: endpointNodeID(component.id, 'out', port.name),
      type: 'entityNode',
      position: { x: 0, y: 0 },
      data: {
        kind: 'port',
        portRole: 'out',
        label: port.name,
        subtitle: port.type,
        showMeta,
      },
    })
  }

  return result
}

function componentDetailEdges(component: Component): Edge[] {
  const result: Edge[] = []
  const inferredSelectorSourcePort = inferSelectorSourceInPort(component)
  for (const connection of component.connections) {
    const sender = connection.sender
    const senderHasSelector = (sender.selector?.length ?? 0) > 0
    const senderMissingEndpoint = !(sender.node && sender.node.trim()) && !(sender.port && sender.port.trim())
    const source = senderHasSelector && senderMissingEndpoint && inferredSelectorSourcePort
      ? endpointNodeID(component.id, 'in', inferredSelectorSourcePort)
      : resolveNodeID(component, sender)
    const target = resolveNodeID(component, connection.receiver)
    if (!source || !target) continue
    result.push({
      id: connection.id,
      type: 'straight',
      source,
      target,
      sourceHandle: connection.sender?.node && connection.sender.node !== 'in' && connection.sender.node !== 'out'
        ? handleIDForPort(endpointPortName(component, connection.sender, 'out'))
        : undefined,
      targetHandle: connection.receiver?.node && connection.receiver.node !== 'in' && connection.receiver.node !== 'out'
        ? handleIDForPort(endpointPortName(component, connection.receiver, 'in'))
        : undefined,
      label: selectorLabel(connection.sender),
    })
  }

  for (const node of component.nodes) {
    const inferredInPort = inferImplicitPortName(component, node.name, 'in')
    if (shouldAddImplicitInputEdge(component, node.name, inferredInPort)) {
      result.push({
        id: `${component.id}/implicit_in/${inferredInPort}->${node.name}`,
        type: 'straight',
        source: endpointNodeID(component.id, 'in', inferredInPort),
        target: endpointNodeID(component.id, node.name),
        targetHandle: handleIDForPort(inferredInPort),
      })
    }

    if (!shouldAddImplicitErrEdge(component, node.name)) {
      continue
    }
    result.push({
      id: `${component.id}/implicit_err/${node.name}`,
      type: 'straight',
      source: endpointNodeID(component.id, node.name),
      target: endpointNodeID(component.id, 'out', 'err'),
      sourceHandle: handleIDForPort('err'),
    })
  }

  return result
}

async function applyLayout(
  nodes: Node<NodeData>[],
  edges: Edge[],
  direction: 'DOWN' | 'RIGHT' = 'DOWN',
  measuredSizes?: Map<string, { width: number; height: number }>,
): Promise<Node<NodeData>[]> {
  const graph = {
    id: 'root',
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': direction,
      'elk.edgeRouting': 'ORTHOGONAL',
      'elk.spacing.nodeNode': '72',
      'elk.layered.spacing.nodeNodeBetweenLayers': '96',
      'elk.layered.spacing.edgeNodeBetweenLayers': '30',
      'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX',
      'elk.layered.nodePlacement.favorStraightEdges': 'true',
      'elk.layered.nodePlacement.bk.edgeStraightening': 'IMPROVE_STRAIGHTNESS',
    },
    children: nodes.map((node) => ({
      id: node.id,
      width: nodeSize(node, measuredSizes).width,
      height: nodeSize(node, measuredSizes).height,
    })),
    edges: edges.map((edge) => ({ id: edge.id, sources: [edge.source], targets: [edge.target] })),
  }

  const layout = await elk.layout(graph)
  const byID = new Map((layout.children ?? []).map((item) => [item.id, item]))

  return nodes.map((node) => {
    const placed = byID.get(node.id)
    return {
      ...node,
      position: {
        x: placed?.x ?? 0,
        y: placed?.y ?? 0,
      },
    }
  })
}

function handleOffsetX(node: Node<NodeData>, handleID: string | null | undefined, role: 'source' | 'target'): number {
  const size = nodeSize(node)
  if (!handleID || node.data.kind !== 'entity') {
    return size.width / 2
  }

  const ports = role === 'source' ? (node.data.outPorts ?? []) : (node.data.inPorts ?? [])
  const portName = handleID.replace(/^port:/u, '')
  const index = ports.findIndex((port) => port.name === portName)
  if (index < 0) {
    return size.width / 2
  }
  return ((index + 0.5) * size.width) / ports.length
}

function nodeHandleX(node: Node<NodeData>, handleID: string | null | undefined, role: 'source' | 'target'): number {
  return node.position.x + handleOffsetX(node, handleID, role)
}

function moveNodeHandleToX(node: Node<NodeData>, targetX: number, handleID: string | null | undefined, role: 'source' | 'target'): Node<NodeData> {
  return {
    ...node,
    position: {
      ...node.position,
      x: targetX - handleOffsetX(node, handleID, role),
    },
  }
}

function straightenMainVerticalChain(nodes: Node<NodeData>[]): Node<NodeData>[] {
  const find = (suffix: string) => nodes.find((n) => n.id.endsWith(suffix))
  const start = find('::in::start')
  const lit = nodes.find((n) => n.data.kind === 'const')
  const call = nodes.find((n) => n.id.includes('::node::') && n.data.kind === 'entity')
  if (!start || !lit || !call) {
    return nodes
  }

  const anchorX = nodeHandleX(call, undefined, 'target')
  return nodes.map((node) => {
    if (node.id === start.id || node.id === lit.id) {
      return moveNodeHandleToX(node, anchorX, undefined, 'source')
    }
    return node
  })
}

function reduceBranchCrossings(nodes: Node<NodeData>[], edges: Edge[]): Node<NodeData>[] {
  const call = nodes.find((n) => n.id.includes('::node::') && n.data.kind === 'entity')
  if (!call) {
    return nodes
  }
  const outPorts = call.data.outPorts ?? []
  if (outPorts.length < 2) {
    return nodes
  }

  const portIndex = new Map(outPorts.map((port, index) => [handleIDForPort(port.name), index]))
  const branchTargets = edges
    .filter((edge) => edge.source === call.id && edge.sourceHandle && edge.target !== call.id)
    .map((edge) => ({ target: edge.target, index: portIndex.get(edge.sourceHandle || '') ?? Number.MAX_SAFE_INTEGER }))
    .filter((item) => item.index !== Number.MAX_SAFE_INTEGER)
    .sort((a, b) => a.index - b.index)

  if (branchTargets.length < 2) {
    return nodes
  }

  const targetX = new Map<string, number>()
  for (const branchTarget of branchTargets) {
    const portName = outPorts[branchTarget.index]?.name
    if (!portName) continue
    targetX.set(branchTarget.target, nodeHandleX(call, handleIDForPort(portName), 'source'))
  }

  return nodes.map((node) => {
    const x = targetX.get(node.id)
    if (x === undefined) {
      return node
    }
    return moveNodeHandleToX(node, x, undefined, 'target')
  })
}

function normalizeLayoutOrigin(nodes: Node<NodeData>[], min = 12): Node<NodeData>[] {
  if (nodes.length === 0) {
    return nodes
  }

  const minX = Math.min(...nodes.map((node) => node.position.x))
  const minY = Math.min(...nodes.map((node) => node.position.y))
  const dx = minX < min ? min - minX : 0
  const dy = minY < min ? min - minY : 0
  if (dx === 0 && dy === 0) {
    return nodes
  }

  return nodes.map((node) => ({
    ...node,
    position: {
      x: node.position.x + dx,
      y: node.position.y + dy,
    },
  }))
}

function isCallNode(node: Node<NodeData>): boolean {
  return node.id.includes('::node::') && node.data.kind === 'entity'
}

function alignTargetToSource(
  target: Node<NodeData>,
  source: Node<NodeData>,
  edge: Edge,
): Node<NodeData> {
  return moveNodeHandleToX(target, nodeHandleX(source, edge.sourceHandle, 'source'), edge.targetHandle, 'target')
}

function incomingEdges(nodeID: string, edges: Edge[]): Edge[] {
  return edges.filter((edge) => edge.target === nodeID)
}

function outgoingEdges(nodeID: string, edges: Edge[]): Edge[] {
  return edges.filter((edge) => edge.source === nodeID)
}

function handleFromConnectionID(edgeID: string, side: 'source' | 'target'): string | undefined {
  const match = side === 'source'
    ? edgeID.match(/\/connection\/port_[^_]+_([^.]*)\./u)
    : edgeID.match(/->port_[^_]+_([^.|]*)/u)
  const port = match?.[1]
  return port ? handleIDForPort(port) : undefined
}

function sourceHandle(edge: Edge): string | undefined {
  return edge.sourceHandle ?? handleFromConnectionID(edge.id, 'source')
}

function targetHandle(edge: Edge): string | undefined {
  return edge.targetHandle ?? handleFromConnectionID(edge.id, 'target')
}

function handleExistsOnNode(node: Node<NodeData>, handleID: string | undefined, role: 'source' | 'target'): boolean {
  if (!handleID || node.data.kind !== 'entity') {
    return true
  }
  const portName = handleID.replace(/^port:/u, '')
  const ports = role === 'source' ? (node.data.outPorts ?? []) : (node.data.inPorts ?? [])
  return ports.some((port) => port.name === portName)
}

function sourceHandleForNode(edge: Edge, node: Node<NodeData>): string | undefined {
  const handle = edge.sourceHandle ?? undefined
  return handleExistsOnNode(node, handle, 'source') ? handle : handleFromConnectionID(edge.id, 'source')
}

function targetHandleForNode(edge: Edge, node: Node<NodeData>): string | undefined {
  const handle = edge.targetHandle ?? undefined
  return handleExistsOnNode(node, handle, 'target') ? handle : handleFromConnectionID(edge.id, 'target')
}

function sortedBySourceHandle(edges: Edge[], source: Node<NodeData>): Edge[] {
  return [...edges].sort((a, b) => {
    const ax = handleOffsetX(source, sourceHandle(a), 'source')
    const bx = handleOffsetX(source, sourceHandle(b), 'source')
    return ax - bx
  })
}

function callDepths(calls: Node<NodeData>[], edges: Edge[]): Map<string, number> {
  const callIDs = new Set(calls.map((node) => node.id))
  const depth = new Map(calls.map((node) => [node.id, 0]))
  for (let pass = 0; pass < calls.length; pass += 1) {
    let changed = false
    for (const edge of edges) {
      if (!callIDs.has(edge.source) || !callIDs.has(edge.target)) continue
      const nextDepth = (depth.get(edge.source) ?? 0) + 1
      if (nextDepth > (depth.get(edge.target) ?? 0)) {
        depth.set(edge.target, nextDepth)
        changed = true
      }
    }
    if (!changed) break
  }
  return depth
}

export function layoutComponentPipeline(nodes: Node<NodeData>[], edges: Edge[]): Node<NodeData>[] | null {
  const calls = nodes.filter(isCallNode)
  if (calls.length === 0 || calls.length > 8) {
    return null
  }

  const nodeByID = new Map(nodes.map((node) => [node.id, node]))
  const depths = callDepths(calls, edges)
  const orderedCalls = [...calls].sort((a, b) => {
    const byDepth = (depths.get(a.id) ?? 0) - (depths.get(b.id) ?? 0)
    return byDepth || a.data.label.localeCompare(b.data.label)
  })
  const next = new Map(nodes.map((node) => [node.id, { ...node, position: { ...node.position } }]))
  const baseX = 260
  const firstCallY = 330
  const callGapY = 250
  const topGapY = 150
  const sinkGapY = 150

  for (const [index, call] of orderedCalls.entries()) {
    const placed = next.get(call.id)
    if (!placed) continue

    const parentEdge = incomingEdges(call.id, edges)
      .map((edge) => ({ edge, source: next.get(edge.source) }))
      .find((item): item is { edge: Edge; source: Node<NodeData> } => Boolean(item.source && isCallNode(item.source)))

    placed.position.y = firstCallY + index * callGapY
    if (parentEdge) {
      placed.position.x = alignTargetToSource(placed, parentEdge.source, {
        ...parentEdge.edge,
        sourceHandle: sourceHandle(parentEdge.edge),
        targetHandle: targetHandle(parentEdge.edge),
      }).position.x
    } else {
      placed.position.x = baseX
    }
  }

  for (const call of orderedCalls) {
    const placedCall = next.get(call.id)
    if (!placedCall) continue
    const callIncoming = incomingEdges(call.id, edges)
      .map((edge) => ({ edge, source: next.get(edge.source) }))
      .filter((item): item is { edge: Edge; source: Node<NodeData> } => Boolean(item.source))
      .filter(({ source }) => !isCallNode(source))
      .sort((a, b) => handleOffsetX(placedCall, a.edge.targetHandle, 'target') - handleOffsetX(placedCall, b.edge.targetHandle, 'target'))

    for (const [incomingIndex, { edge, source }] of callIncoming.entries()) {
      const fallbackPort = placedCall.data.inPorts?.[incomingIndex]?.name
      const effectiveTargetHandle = targetHandleForNode(edge, placedCall) ?? (fallbackPort ? handleIDForPort(fallbackPort) : undefined)
      const sourceX = nodeHandleX(placedCall, effectiveTargetHandle, 'target')
      source.position = {
        ...moveNodeHandleToX(source, sourceX, sourceHandleForNode(edge, source), 'source').position,
        y: placedCall.position.y - topGapY,
      }
    }
  }

  for (const node of Array.from(next.values()).filter((item) => item.id.includes('::in::'))) {
    const children = outgoingEdges(node.id, edges)
      .map((edge) => next.get(edge.target))
      .filter((item): item is Node<NodeData> => Boolean(item))
    if (children.length === 0) continue
    const centerX = children.reduce((sum, child) => sum + nodeHandleX(child, undefined, 'target'), 0) / children.length
    node.position = {
      x: centerX - handleOffsetX(node, undefined, 'source'),
      y: Math.min(...children.map((child) => child.position.y)) - topGapY,
    }
  }

  for (const call of orderedCalls) {
    const placedCall = next.get(call.id)
    if (!placedCall) continue
    const terminalEdges = sortedBySourceHandle(outgoingEdges(call.id, edges), placedCall)
      .map((edge) => ({ edge, target: next.get(edge.target) }))
      .filter((item): item is { edge: Edge; target: Node<NodeData> } => Boolean(item.target))
      .filter(({ target }) => !isCallNode(target) || outgoingEdges(target.id, edges).length === 0)

    const occupied = new Set<string>()
    for (const { edge, target } of terminalEdges) {
      if (isCallNode(target) && incomingEdges(target.id, edges).some((incoming) => isCallNode(nodeByID.get(incoming.source) as Node<NodeData>))) {
        continue
      }
      const targetY = placedCall.position.y + fallbackNodeHeight(placedCall) + sinkGapY
      const aligned = alignTargetToSource(target, placedCall, { ...edge, sourceHandle: sourceHandle(edge), targetHandle: targetHandle(edge) })
      const slot = `${Math.round(aligned.position.x)}:${targetY}`
      const bump = occupied.has(slot) ? SNAP_GRID_STEPS[2] * occupied.size : 0
      occupied.add(slot)
      target.position = {
        x: aligned.position.x + bump,
        y: targetY,
      }
    }
  }

  return normalizeLayoutOrigin(nodes.map((node) => next.get(node.id) ?? node))
}

export function shouldUseMeasuredLayoutSizes(route: Route): boolean {
  return route.kind === 'entity'
}

type PersistedLayout = {
  signature: string
  positions: Record<string, { x: number; y: number }>
}

function routeLayoutKey(route: Route): string {
  return `neva-lsp:layout:v1:${routeToHash(route)}`
}

function nodeSignature(nodes: Node<NodeData>[]): string {
  return nodes.map((node) => node.id).sort().join('|')
}

function loadPersistedLayout(key: string): PersistedLayout | null {
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return null
    return JSON.parse(raw) as PersistedLayout
  } catch {
    return null
  }
}

function savePersistedLayout(key: string, payload: PersistedLayout) {
  try {
    window.localStorage.setItem(key, JSON.stringify(payload))
  } catch {
    // Ignore storage errors to avoid blocking graph interactions.
  }
}

function readMeasuredNodeSizes(nodeIDs: string[], zoom = 1): Map<string, { width: number; height: number }> {
  const result = new Map<string, { width: number; height: number }>()
  const scale = zoom > 0 ? zoom : 1
  for (const id of nodeIDs) {
    const escaped = typeof CSS !== 'undefined' && CSS.escape ? CSS.escape(id) : id.replace(/"/g, '\\"')
    const element = document.querySelector(`.react-flow__node[data-id="${escaped}"]`) as HTMLElement | null
    if (!element) continue
    const rect = element.getBoundingClientRect()
    if (rect.width > 0 && rect.height > 0) {
      result.set(id, { width: rect.width / scale, height: rect.height / scale })
    }
  }
  return result
}

export function GraphCanvas({
  modules,
  route,
  file,
  breadcrumbs,
  canGoBack,
  canGoForward,
  onGoBack,
  onGoForward,
  onNavigate,
  onResolveOpen,
}: Props) {
  const [nodes, setNodes] = useState<Node<NodeData>[]>([])
  const [edges, setEdges] = useState<Edge[]>([])
  const [flow, setFlow] = useState<ReactFlowInstance<Node<NodeData>, Edge> | null>(null)
  const [layoutVersion, setLayoutVersion] = useState(0)
  const [copyDone, setCopyDone] = useState(false)
  const [interactive, setInteractive] = useState(true)
  const [layoutKey, setLayoutKey] = useState<string | null>(null)
  const [layoutSignature, setLayoutSignature] = useState('')
  const [layoutSeed, setLayoutSeed] = useState(0)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [snapGridLevel, setSnapGridLevel] = useState(() => {
    if (typeof window === 'undefined') return 0
    const saved = window.localStorage.getItem(SNAP_GRID_STORAGE_KEY)
    if (saved === '0' || saved === '1' || saved === '2') {
      return Number(saved)
    }
    if (saved === 'true') {
      return 2
    }
    return 0
  })
  const [theme, setTheme] = useState<'light' | 'dark'>(() => {
    if (typeof window !== 'undefined') {
      const saved = window.localStorage.getItem(THEME_STORAGE_KEY)
      if (saved === 'dark' || saved === 'light') {
        return saved
      }
    }
    if (typeof window !== 'undefined' && window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
      return 'dark'
    }
    return 'light'
  })

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, theme)
    } catch {
      // Ignore storage errors.
    }
  }, [theme])

  useEffect(() => {
    try {
      window.localStorage.setItem(SNAP_GRID_STORAGE_KEY, String(snapGridLevel))
    } catch {
      // Ignore storage errors.
    }
  }, [snapGridLevel])

  const snapGridSize = SNAP_GRID_STEPS[snapGridLevel] ?? 0
  const snapToGrid = snapGridSize > 0
  const snapGrid: [number, number] = [snapGridSize || 1, snapGridSize || 1]

  useEffect(() => {
    if (!flow || nodes.length === 0 || layoutVersion === 0) {
      return
    }
    const frame = window.requestAnimationFrame(() => {
      flow.fitView({ duration: 0, padding: 0.2 })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [flow, layoutVersion, nodes.length])

  useEffect(() => {
    let canceled = false

    async function run() {
      const useMeasuredLayout = shouldUseMeasuredLayoutSizes(route)
      if (useMeasuredLayout && !flow) {
        return
      }

      let nextNodes: Node<NodeData>[] = []
      let nextEdges: Edge[] = []
      let direction: 'DOWN' | 'RIGHT' = 'DOWN'

      if (route.kind === 'modules') {
        nextNodes = moduleNodes(modules)
      }

      if (route.kind === 'module') {
        nextNodes = packageNodes(modules, route.modulePath)
      }

      if (route.kind === 'package') {
        nextNodes = fileNodes(modules, route.modulePath, route.packageName)
      }

      if (route.kind === 'file') {
        if (!file) {
          setNodes([])
          setEdges([])
          return
        }
        nextNodes = fileEntityNodes(file)
      }

      if (route.kind === 'entity') {
        if (!file) {
          setNodes([])
          setEdges([])
          return
        }

        const component = file.components.find((item) => item.id === route.entityId)
        if (component && canDrillComponent(component)) {
          nextNodes = componentDetailNodes(component, true, (target) => {
            onNavigate({ kind: 'entity', fileId: target.fileId, entityId: target.entityId }, true)
          })
          nextEdges = componentDetailEdges(component)
          direction = 'DOWN'
        } else {
          nextNodes = fileEntityNodes(file, route.entityId)
        }
      }

      const measured = useMeasuredLayout
        ? readMeasuredNodeSizes(nextNodes.map((node) => node.id), flow?.getZoom() ?? 1)
        : new Map<string, { width: number; height: number }>()
      let laidOut = await applyLayout(
        nextNodes,
        nextEdges,
        direction,
        measured.size > 0 ? measured : undefined,
      )
      if (route.kind === 'entity') {
        laidOut = layoutComponentPipeline(laidOut, nextEdges) ?? normalizeLayoutOrigin(reduceBranchCrossings(straightenMainVerticalChain(laidOut), nextEdges))
      }
      if (!canceled) {
        const key = routeLayoutKey(route)
        const signature = nodeSignature(laidOut)
        const saved = layoutSeed === 0 ? loadPersistedLayout(key) : null
        const withSavedPositions = saved && saved.signature === signature
          ? laidOut.map((node) => ({
            ...node,
            position: saved.positions[node.id] ?? node.position,
          }))
          : laidOut

        setNodes(withSavedPositions)
        setEdges(nextEdges)
        setLayoutKey(key)
        setLayoutSignature(signature)
        setLayoutVersion((v) => v + 1)
      }
    }

    void run()
    return () => {
      canceled = true
    }
  }, [modules, route, file, layoutSeed, flow])

  async function copyCurrentURL() {
    try {
      await navigator.clipboard.writeText(window.location.href)
      setCopyDone(true)
      window.setTimeout(() => setCopyDone(false), 1200)
    } catch {
      setCopyDone(false)
    }
  }

  function routeDown(current: Route, node: Node<NodeData>): Route | null {
    if (current.kind === 'modules' && node.data.modulePath) {
      return { kind: 'module', modulePath: node.data.modulePath }
    }
    if (current.kind === 'module' && node.data.modulePath && node.data.packageName) {
      return { kind: 'package', modulePath: node.data.modulePath, packageName: node.data.packageName }
    }
    if (current.kind === 'package' && node.data.fileId) {
      return { kind: 'file', fileId: node.data.fileId }
    }
    if (current.kind === 'file' && node.data.fileId && node.data.entityId) {
      if (file && node.data.navType === 'component' && isNativeComponent(file, node.data.entityId)) {
        return null
      }
      return { kind: 'entity', fileId: node.data.fileId, entityId: node.data.entityId }
    }
    return null
  }

  function handleNodesChange(changes: NodeChange<Node<NodeData>>[]) {
    setNodes((current) => applyNodeChanges(changes, current))
  }

  function persistCurrentLayout(nextNodes: Node<NodeData>[]) {
    if (!layoutKey || !layoutSignature) {
      return
    }
    const positions: Record<string, { x: number; y: number }> = {}
    for (const node of nextNodes) {
      positions[node.id] = { x: node.position.x, y: node.position.y }
    }
    savePersistedLayout(layoutKey, { signature: layoutSignature, positions })
  }

  function resetCurrentLayout() {
    if (layoutKey) {
      try {
        window.localStorage.removeItem(layoutKey)
      } catch {
        // Ignore storage errors.
      }
    }
    setLayoutSeed((seed) => seed + 1)
  }

  return (
    <section className="canvas-shell">
      <div className="canvas-overlay">
        <div className="canvas-nav-buttons">
          <div className="canvas-nav-left">
            <button onClick={onGoBack} disabled={!canGoBack}>←</button>
            <button onClick={onGoForward} disabled={!canGoForward}>→</button>
          </div>
          <div className="canvas-nav-right">
            <button
              className="canvas-settings-toggle"
              onClick={() => setSettingsOpen(true)}
              title="Open graph settings"
              aria-label="Open graph settings"
            >
              ⚙
            </button>
          </div>
        </div>
        <div className="canvas-breadcrumbs">
          <div className="canvas-breadcrumbs-links">
            {breadcrumbs.map((crumb, index) => (
              <span key={crumb.key}>
                <button className="breadcrumb-link" onClick={() => onNavigate(crumb.route, true)}>{crumb.label}</button>
                {index < breadcrumbs.length - 1 ? <span className="breadcrumb-sep"> / </span> : null}
              </span>
            ))}
          </div>
          <button
            className="canvas-copy-url"
            onClick={() => void copyCurrentURL()}
            title="Copy URL"
            aria-label="Copy URL"
          >
            {copyDone ? '✅' : '📋'}
          </button>
        </div>
      </div>
      {settingsOpen ? (
        <div className="canvas-settings-backdrop" role="presentation" onMouseDown={() => setSettingsOpen(false)}>
          <div
            className="canvas-settings-panel"
            role="dialog"
            aria-modal="true"
            aria-label="Graph settings"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <div className="canvas-settings-header">
              <div>
                <div className="canvas-settings-title">Graph settings</div>
                <div className="canvas-settings-subtitle">Layout behavior is stored locally per browser.</div>
              </div>
              <button type="button" onClick={() => setSettingsOpen(false)} aria-label="Close graph settings">×</button>
            </div>
            <label className="canvas-settings-row">
              <span>
                <span className="canvas-settings-label">Theme</span>
                <span className="canvas-settings-hint">Stored locally per browser.</span>
              </span>
              <span className="canvas-settings-actions">
                <button
                  type="button"
                  className={theme === 'light' ? 'canvas-settings-choice is-active' : 'canvas-settings-choice'}
                  onClick={() => setTheme('light')}
                >
                  Light
                </button>
                <button
                  type="button"
                  className={theme === 'dark' ? 'canvas-settings-choice is-active' : 'canvas-settings-choice'}
                  onClick={() => setTheme('dark')}
                >
                  Dark
                </button>
              </span>
            </label>
            <label className="canvas-settings-row">
              <span>
                <span className="canvas-settings-label">Snap nodes to grid</span>
                <span className="canvas-settings-hint">Three levels: off, 12 px, 24 px.</span>
              </span>
              <span className="canvas-settings-slider-wrap">
                <input
                  type="range"
                  min="0"
                  max="2"
                  step="1"
                  value={snapGridLevel}
                  onChange={(event) => setSnapGridLevel(Number(event.target.value))}
                />
                <span className="canvas-settings-slider-value">{snapGridLabel(snapGridLevel)}</span>
              </span>
            </label>
          </div>
        </div>
      ) : null}

      <ReactFlow
        nodes={nodes}
        edges={edges}
        defaultEdgeOptions={{
          type: 'straight',
          style: { strokeWidth: 1.5 },
        }}
        nodeTypes={nodeTypes}
        nodesDraggable={interactive}
        elementsSelectable={interactive}
        panOnDrag={interactive}
        snapToGrid={snapToGrid}
        snapGrid={snapGrid}
        fitView
        onInit={setFlow}
        onNodesChange={handleNodesChange}
        onNodeDragStop={(_, draggedNode) => {
          const updatedNodes = nodes.map((node) => (node.id === draggedNode.id ? { ...node, position: draggedNode.position } : node))
          persistCurrentLayout(updatedNodes)
        }}
        onNodeClick={(_, node) => {
          const nextRoute = routeDown(route, node)
          if (nextRoute && !(route.kind === 'file' && file && node.data.entityId && isNativeComponent(file, node.data.entityId))) {
            onNavigate(nextRoute, true)
            return
          }
          if (file && node.data.navType === 'component' && node.data.entityId && isNativeComponent(file, node.data.entityId)) {
            return
          }
          if (route.kind === 'entity' && node.data.fileId && node.data.entityId) {
            void onResolveOpen({ fileId: node.data.fileId, entityId: node.data.entityId })
          }
        }}
      >
        <MiniMap
          key={`minimap-${theme}`}
          pannable
          zoomable
          nodeClassName="minimap-node"
          nodeColor={(node) => minimapNodeFill(node as Node<NodeData>, theme)}
          nodeStrokeColor={theme === 'dark' ? '#d6dbe3' : '#3f4650'}
          nodeStrokeWidth={2}
          nodeBorderRadius={4}
          bgColor={theme === 'dark' ? '#3b414c' : '#d3d8e0'}
          maskColor={theme === 'dark' ? 'rgba(8, 10, 14, 0.22)' : 'rgba(255, 255, 255, 0.22)'}
          maskStrokeColor={theme === 'dark' ? '#e1e6ef' : '#3f4650'}
          maskStrokeWidth={1.25}
        />
        <Controls
          showInteractive
          onInteractiveChange={(value) => setInteractive(value)}
        >
          <ControlButton
            onClick={resetCurrentLayout}
            title="Reset saved layout"
            aria-label="Reset saved layout"
          >
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M4 4h6v6H4V4Zm2 2v2h2V6H6Zm8-2h6v6h-6V4Zm2 2v2h2V6h-2ZM4 14h6v6H4v-6Zm2 2v2h2v-2H6Zm9-1h2v2h-2v-2Zm3 3h2v2h-2v-2Zm-4 0h2v2h-2v-2Zm4-4h2v2h-2v-2Z" />
            </svg>
          </ControlButton>
        </Controls>
        <Background />
      </ReactFlow>
    </section>
  )
}
