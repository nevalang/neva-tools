import { describe, expect, it } from 'vitest'
import { endpointPortName, inferImplicitPortName, parseSignaturePorts, shouldAddImplicitErrEdge, shouldAddImplicitInputEdge } from './graphSemantics'
import type { Component } from './types'

function baseComponent(): Component {
  return {
    id: 'c1',
    name: 'Println',
    inPorts: [{ name: 'data', type: 'T' }],
    outPorts: [{ name: 'res', type: 'T' }, { name: 'err', type: 'error' }],
    nodes: [{ id: 'n1', name: 'println', resolvedRef: { canonicalRef: 'std/fmt/Println', fileId: 'f', entityId: 'e', entityKind: 'component_entity', anchor: { text: 'Println<T>(dataT)(resT,errerror)' } }, errGuard: true }],
    connections: [],
  }
}

describe('graphSemantics', () => {
  it('parses signature ports with known names', () => {
    const parsed = parseSignaturePorts('Println<T>(dataT)(resT,errerror)', ['data'], ['res', 'err'])
    expect(parsed.in.get('data')).toBe('T')
    expect(parsed.out.get('res')).toBe('T')
    expect(parsed.out.get('err')).toBe('error')
  })

  it('splits compact generic output ports using signature type parameters', () => {
    const parsed = parseSignaturePorts('Struct<Tstruct{}>()(resT)', [], [])
    expect(Array.from(parsed.out.entries())).toEqual([['res', 'T']])
  })

  it('infers implicit output as res for errGuard nodes', () => {
    const c = baseComponent()
    expect(inferImplicitPortName(c, 'println', 'out')).toBe('res')
  })

  it('uses inferred port when endpoint port is empty', () => {
    const c = baseComponent()
    expect(endpointPortName(c, { node: 'println', port: '' }, 'out')).toBe('res')
    expect(endpointPortName(c, { node: 'println', port: '' }, 'in')).toBe('data')
  })

  it('adds implicit input and err edges when missing', () => {
    const c = baseComponent()
    expect(shouldAddImplicitInputEdge(c, 'println', 'data')).toBe(true)
    expect(shouldAddImplicitErrEdge(c, 'println')).toBe(true)
  })

  it('does not add implicit edges when explicit edges exist', () => {
    const c = baseComponent()
    c.connections.push(
      { id: '1', sender: { node: 'in', port: 'data' }, receiver: { node: 'println', port: 'data' } },
      { id: '2', sender: { node: 'println', port: 'err' }, receiver: { node: 'out', port: 'err' } },
    )
    expect(shouldAddImplicitInputEdge(c, 'println', 'data')).toBe(false)
    expect(shouldAddImplicitErrEdge(c, 'println')).toBe(false)
  })
})
