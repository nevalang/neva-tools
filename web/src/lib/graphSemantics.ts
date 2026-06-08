import type { Component, FileView } from './types'

function parseSignatureTypeParamNames(signatureText: string | undefined): string[] {
  if (!signatureText) {
    return []
  }
  const normalized = signatureText.replace(/\s+/g, '')
  const generic = normalized.match(/^[A-Za-z_][A-Za-z0-9_.]*<([^()]*)>\(/)
  if (!generic?.[1]) {
    return []
  }
  return Array.from(generic[1].matchAll(/([A-Z][A-Za-z0-9_]*?)(?=(?:struct|stream|error|string|float|bool|int|any|[{},]|$))/g))
    .map((match) => match[1])
    .filter((name): name is string => Boolean(name))
}

export function parsePortList(raw: string, knownNames: string[], typeParamNames: string[] = []): Map<string, string> {
  const result = new Map<string, string>()
  const known = [...knownNames].sort((a, b) => b.length - a.length)
  const compactTypeStarts = [
    ...typeParamNames.sort((a, b) => b.length - a.length),
    'stream<',
    'error',
    'string',
    'float',
    'bool',
    'int',
    'any',
  ]

  function splitCompactToken(item: string): { name: string; type: string } | null {
    for (const marker of compactTypeStarts) {
      const idx = item.indexOf(marker)
      if (idx > 0) {
        return { name: item.slice(0, idx), type: item.slice(idx) }
      }
    }
    return null
  }

  for (const chunk of raw.split(',')) {
    const item = chunk.trim()
    if (!item) continue
    let name = ''
    for (const candidate of known) {
      if (item.startsWith(candidate)) {
        name = candidate
        break
      }
    }
    if (!name) {
      const fallback = item.match(/^([A-Za-z_][A-Za-z0-9_]*)(.*)$/)
      if (!fallback) continue
      const compact = splitCompactToken(item)
      if (compact) {
        name = compact.name
        result.set(name, compact.type.trim())
        continue
      }
      name = fallback[1]
    }
    const type = item.slice(name.length).trim()
    result.set(name, type)
  }
  return result
}

export function parseSignaturePorts(
  signatureText: string | undefined,
  knownInNames: string[],
  knownOutNames: string[],
): { in: Map<string, string>; out: Map<string, string> } {
  if (!signatureText) {
    return { in: new Map<string, string>(), out: new Map<string, string>() }
  }
  const normalized = signatureText.replace(/\s+/g, '')
  const pair = normalized.match(/\(([^()]*)\)\(([^()]*)\)/)
  if (!pair) {
    return { in: new Map<string, string>(), out: new Map<string, string>() }
  }
  const typeParamNames = parseSignatureTypeParamNames(signatureText)
  return {
    in: parsePortList(pair[1] ?? '', knownInNames, typeParamNames),
    out: parsePortList(pair[2] ?? '', knownOutNames, typeParamNames),
  }
}

function parseSignaturePortNames(
  signatureText: string | undefined,
  direction: 'in' | 'out',
  knownNames: string[],
): string[] {
  if (!signatureText) {
    return []
  }
  const normalized = signatureText.replace(/\s+/g, '')
  const pair = normalized.match(/\(([^()]*)\)\(([^()]*)\)/)
  if (!pair) {
    return []
  }
  const raw = direction === 'in' ? pair[1] : pair[2]
  if (!raw) {
    return []
  }
  const parsed = parsePortList(raw, knownNames, parseSignatureTypeParamNames(signatureText))
  if (parsed.size > 0) {
    return Array.from(parsed.keys())
  }

  const fallbackNames: string[] = []
  for (const chunk of raw.split(',')) {
    const item = chunk.trim()
    if (!item) continue
    const m = item.match(/^([A-Za-z_][A-Za-z0-9_]*)/)
    if (!m) continue
    fallbackNames.push(m[1])
  }
  return fallbackNames
}

export function inferImplicitPortName(component: Component, nodeName: string, direction: 'in' | 'out'): string {
  const node = component.nodes.find((item) => item.name === nodeName)
  if (!node) {
    return 'sig'
  }

  const knownNames = direction === 'in'
    ? component.inPorts.map((port) => port.name)
    : component.outPorts.map((port) => port.name)
  for (const connection of component.connections) {
    if (direction === 'in' && connection.receiver?.node === nodeName) {
      const explicit = (connection.receiver?.port ?? '').trim()
      if (explicit) knownNames.push(explicit)
    }
    if (direction === 'out' && connection.sender?.node === nodeName) {
      const explicit = (connection.sender?.port ?? '').trim()
      if (explicit) knownNames.push(explicit)
    }
  }

  const parsedNames = parseSignaturePortNames(node.resolvedRef?.anchor?.text, direction, knownNames)
  if (parsedNames.length === 0) {
    return 'sig'
  }

  const filtered = direction === 'out' && node.errGuard
    ? parsedNames.filter((name) => name !== 'err')
    : parsedNames

  if (filtered.length === 1) {
    return filtered[0]
  }
  if (direction === 'in' && filtered.includes('data')) {
    return 'data'
  }
  if (direction === 'out' && filtered.includes('res')) {
    return 'res'
  }
  if (filtered.includes('sig')) {
    return 'sig'
  }
  return filtered[0] ?? parsedNames[0] ?? 'sig'
}

export function endpointPortName(
  component: Component,
  endpoint: { node?: string; port?: string },
  direction: 'in' | 'out',
): string {
  const explicitPort = (endpoint.port ?? '').trim()
  if (explicitPort !== '') {
    return explicitPort
  }
  const nodeName = endpoint.node ?? ''
  if (!nodeName || nodeName === 'in' || nodeName === 'out') {
    return 'sig'
  }
  return inferImplicitPortName(component, nodeName, direction)
}

function hasIncomingEdge(component: Component, nodeName: string, inPortName: string): boolean {
  for (const connection of component.connections) {
    if ((connection.receiver?.node ?? '') !== nodeName) {
      continue
    }
    if (endpointPortName(component, connection.receiver, 'in') === inPortName) {
      return true
    }
  }
  return false
}

function hasOutgoingFromComponentIn(component: Component, inPortName: string): boolean {
  for (const connection of component.connections) {
    if ((connection.sender?.node ?? '') !== 'in') {
      continue
    }
    const senderPort = (connection.sender?.port ?? '').trim()
    if (senderPort === inPortName) {
      return true
    }
  }
  return false
}

export function shouldAddImplicitInputEdge(component: Component, nodeName: string, inPortName: string): boolean {
  if (!component.inPorts.some((port) => port.name === inPortName)) {
    return false
  }
  if (hasIncomingEdge(component, nodeName, inPortName)) {
    return false
  }
  if (hasOutgoingFromComponentIn(component, inPortName)) {
    return false
  }
  return true
}

export function shouldAddImplicitErrEdge(component: Component, nodeName: string): boolean {
  const node = component.nodes.find((item) => item.name === nodeName)
  if (!node?.errGuard) {
    return false
  }
  if (!component.outPorts.some((port) => port.name === 'err')) {
    return false
  }
  for (const connection of component.connections) {
    if ((connection.sender?.node ?? '') !== nodeName) continue
    const outPort = endpointPortName(component, connection.sender, 'out')
    if (outPort === 'err') {
      return false
    }
  }
  return true
}

export function isNativeComponent(file: FileView, entityID: string): boolean {
  const component = file.components.find((item) => item.id === entityID)
  if (!component) {
    return false
  }
  return component.nodes.length === 0 && component.connections.length === 0
}
