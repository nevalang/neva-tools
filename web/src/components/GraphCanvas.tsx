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

type NodeData = {
  kind: 'entity' | 'port' | 'nav' | 'const'
  navType?: 'module' | 'package' | 'file' | 'component' | 'interface' | 'type' | 'const'
  portRole?: 'in' | 'out'
  label: string
  subtitle?: string
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
  if (navType === 'module') return theme === 'dark' ? '#7ea8cf' : '#356287'
  if (navType === 'package') return theme === 'dark' ? '#d8ad7f' : '#8c5a2f'
  if (navType === 'file') return theme === 'dark' ? '#93c8a3' : '#326f56'
  if (navType === 'component') return theme === 'dark' ? '#e3ba8d' : '#9a6734'
  if (navType === 'interface') return theme === 'dark' ? '#94b6dd' : '#44698e'
  if (navType === 'type') return theme === 'dark' ? '#86bfd6' : '#2f6f97'
  if (navType === 'const') return theme === 'dark' ? '#e6c58a' : '#8a5a2a'
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
  return modules.map((mod) => ({
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
  return moduleItem.packages.map((pkg) => ({
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
  return pkg.fileSummaries.map((file) => ({
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

function constPreview(item: ConstDecl): string {
  const type = item.type?.trim()
  const value = item.value?.trim()
  if (type && value) return `${type} = ${value}`
  return type || value || item.anchor?.text?.trim() || 'const'
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

function fallbackNodeWidth(node: Node<NodeData>): number {
  if (node.data.kind === 'port') return 88
  if (node.data.kind === 'const') return 92

  const inWidth = (node.data.inPorts ?? []).reduce((sum, port) => sum + estimatedPortPillWidth(port), 0)
  const outWidth = (node.data.outPorts ?? []).reduce((sum, port) => sum + estimatedPortPillWidth(port), 0)
  return Math.max(240, inWidth, outWidth)
}

function fallbackNodeHeight(node: Node<NodeData>): number {
  if (node.data.kind === 'port') return 70
  if (node.data.kind === 'const') return 72
  return 120 + (node.data.diArgs?.length ?? 0) * 30
}

function nodeSize(node: Node<NodeData>, measuredSizes?: Map<string, { width: number; height: number }>): { width: number; height: number } {
  return measuredSizes?.get(node.id) ?? { width: fallbackNodeWidth(node), height: fallbackNodeHeight(node) }
}

function fileEntityNodes(file: FileView, selectedEntityID?: string): Node<NodeData>[] {
  const components = file.components.map((component) => ({
    id: `entity:${component.id}`,
    type: 'entityNode' as const,
    className: `${canDrillComponent(component) ? 'rf-node-clickable ' : ''}rf-node-kind-component${component.id === selectedEntityID ? ' selected' : ''}`,
    position: { x: 0, y: 0 },
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
    data: {
      kind: 'nav' as const,
      navType: 'type' as const,
      label: item.name,
      subtitle: typePreview(item),
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
    data: {
      kind: 'nav' as const,
      navType: 'const' as const,
      label: item.name,
      subtitle: constPreview(item),
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

function layoutSingleCallPipeline(nodes: Node<NodeData>[], edges: Edge[]): Node<NodeData>[] | null {
  const primaryCalls = nodes.filter((node) =>
    node.id.includes('::node::') &&
    node.data.kind === 'entity' &&
    edges.some((edge) => edge.source === node.id),
  )
  const call = primaryCalls[0]
  if (!call || primaryCalls.length !== 1) {
    return null
  }

  const inputPorts = nodes.filter((node) => node.id.includes('::in::'))
  const consts = nodes.filter((node) => node.data.kind === 'const')
  const outputTargets = edges
    .filter((edge) => edge.source === call.id)
    .map((edge) => ({ edge, node: nodes.find((node) => node.id === edge.target) }))
    .filter((item): item is { edge: Edge; node: Node<NodeData> } => Boolean(item.node))

  const callX = 240
  const callY = 340
  const callInputX = callX + handleOffsetX(call, undefined, 'target')
  const next = new Map(nodes.map((node) => [node.id, { ...node, position: { ...node.position } }]))

  const placedCall = next.get(call.id)
  if (placedCall) {
    placedCall.position = { x: callX, y: callY }
  }

  inputPorts.forEach((node, index) => {
    const placed = next.get(node.id)
    if (!placed) return
    placed.position = {
      x: callInputX - handleOffsetX(node, undefined, 'source') + index * 96,
      y: 12,
    }
  })

  consts.forEach((node, index) => {
    const placed = next.get(node.id)
    if (!placed) return
    placed.position = {
      x: callInputX - handleOffsetX(node, undefined, 'source') + index * 108,
      y: 170,
    }
  })

  const outputRows = new Map<string, number>()
  for (const { edge, node } of outputTargets) {
    const placed = next.get(node.id)
    if (!placed) continue
    const sourceX = callX + handleOffsetX(call, edge.sourceHandle, 'source')
    const row = outputRows.get(node.id) ?? outputRows.size
    outputRows.set(node.id, row)
    placed.position = {
      x: sourceX - handleOffsetX(node, edge.targetHandle, 'target'),
      y: callY + fallbackNodeHeight(call) + 130 + row * 120,
    }
  }

  return normalizeLayoutOrigin(nodes.map((node) => next.get(node.id) ?? node))
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
  const [theme, setTheme] = useState<'light' | 'dark'>(() => {
    if (typeof window !== 'undefined' && window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
      return 'dark'
    }
    return 'light'
  })

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
  }, [theme])

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

      const measured = readMeasuredNodeSizes(nextNodes.map((node) => node.id), flow?.getZoom() ?? 1)
      let laidOut = await applyLayout(
        nextNodes,
        nextEdges,
        direction,
        measured.size > 0 ? measured : undefined,
      )
      if (route.kind === 'entity') {
        laidOut = layoutSingleCallPipeline(laidOut, nextEdges) ?? normalizeLayoutOrigin(reduceBranchCrossings(straightenMainVerticalChain(laidOut), nextEdges))
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
              className="canvas-theme-toggle"
              onClick={() => setTheme((current) => current === 'light' ? 'dark' : 'light')}
              title={theme === 'light' ? 'Switch to dark theme' : 'Switch to light theme'}
              aria-label={theme === 'light' ? 'Switch to dark theme' : 'Switch to light theme'}
            >
              {theme === 'light' ? '🌙' : '☀️'}
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
            title="Reset node positions"
            aria-label="Reset node positions"
          >
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M6.3 7.3A8 8 0 1 1 4 13h2a6 6 0 1 0 1.8-4.3L10 11H3V4l3.3 3.3Z" />
            </svg>
          </ControlButton>
        </Controls>
        <Background />
      </ReactFlow>
    </section>
  )
}
