import { describe, expect, it } from 'vitest'
import type { Edge, Node } from '@xyflow/react'
import { fileEntityNodes, layoutComponentPipeline, shouldUseMeasuredLayoutSizes, type NodeData } from './GraphCanvas'
import type { FileView } from '../lib/types'

describe('GraphCanvas helpers', () => {
  it('preserves entity order provided by the API', () => {
    const file: FileView = {
      id: 'module/@/package/p/file/main',
      name: 'main',
      imports: [],
      components: [
        { id: 'c-a', name: 'Alpha', inPorts: [], outPorts: [], nodes: [], connections: [] },
        { id: 'c-z', name: 'Zed', inPorts: [], outPorts: [], nodes: [], connections: [] },
      ],
      interfaces: [{ id: 'i-z', name: 'ZIface', inPorts: [], outPorts: [] }, { id: 'i-a', name: 'AIface', inPorts: [], outPorts: [] }],
      types: [
        { id: 't-a', name: 'AType', type: 'string' },
        { id: 't-z', name: 'ZType', type: 'int' },
      ],
      consts: [{ id: 'k-a', name: 'Answer', type: 'int', value: '42' }, { id: 'k-z', name: 'Zebra', type: 'string', value: '"z"' }],
    }

    const nodes = fileEntityNodes(file)
    expect(nodes.map((node) => node.data.label)).toEqual([
      'Alpha',
      'Zed',
      'ZIface',
      'AIface',
      'AType',
      'ZType',
      'Answer',
      'Zebra',
    ])

    const constNode = nodes.find((node) => node.data.label === 'Answer')
    expect(constNode?.data.subtitle).toBe('const · int')
    expect(constNode?.data.detail).toBe('42')
    expect(constNode?.style?.width).toBe(380)
    const typeNode = nodes.find((node) => node.data.label === 'AType')
    expect(typeNode?.data.subtitle).toBe('type · string')
    expect(typeNode?.style?.width).toBe(300)
  })

  it('uses measured DOM sizes only for entity graphs', () => {
    expect(shouldUseMeasuredLayoutSizes({ kind: 'modules' })).toBe(false)
    expect(shouldUseMeasuredLayoutSizes({ kind: 'module', modulePath: '@' })).toBe(false)
    expect(shouldUseMeasuredLayoutSizes({ kind: 'package', modulePath: '@', packageName: 'const_refs' })).toBe(false)
    expect(shouldUseMeasuredLayoutSizes({ kind: 'file', fileId: 'module/@/package/const_refs/file/main' })).toBe(false)
    expect(shouldUseMeasuredLayoutSizes({ kind: 'entity', fileId: 'module/@/package/const_refs/file/main', entityId: 'module/@/package/const_refs/file/main/component/Main@0' })).toBe(true)
  })

  it('lays out a two-call component pipeline by port order', () => {
    const nodes: Node<NodeData>[] = [
      portNode('component/Main@0::in::start', 'in', 'start'),
      constNode('component/Main@0::const::int::32', '32', 'int'),
      constNode('component/Main@0::const::string::John', '"John"', 'string'),
      callNode('component/Main@0::node::builder', 'builder', [{ name: 'age', type: '' }, { name: 'name', type: '' }], [{ name: 'resT', type: '' }]),
      callNode('component/Main@0::node::println', 'println', [{ name: 'data', type: 'T' }], [{ name: 'res', type: 'T' }, { name: 'err', type: 'error' }]),
      portNode('component/Main@0::out::res', 'out', 'stop'),
      callNode('component/Main@0::node::panic', 'panic', [{ name: 'data', type: 'any' }], []),
    ]
    const edges: Edge[] = [
      { id: 'start-32', source: nodes[0].id, target: nodes[1].id },
      { id: 'start-john', source: nodes[0].id, target: nodes[2].id },
      {
        id: 'module/@/package/struct_builder/file/main/component/Main@0/connection/const_int=32.->port_builder_age.|chain_via_chain|depth_1#0',
        source: nodes[1].id,
        target: nodes[3].id,
        targetHandle: 'port:sig',
      },
      {
        id: 'module/@/package/struct_builder/file/main/component/Main@0/connection/const_string=\"John\".->port_builder_name.|chain_via_chain|depth_1#0',
        source: nodes[2].id,
        target: nodes[3].id,
        targetHandle: 'port:sig',
      },
      { id: 'builder-println', source: nodes[3].id, target: nodes[4].id, sourceHandle: 'port:resT', targetHandle: 'port:data' },
      { id: 'println-stop', source: nodes[4].id, target: nodes[5].id, sourceHandle: 'port:res' },
      { id: 'println-panic', source: nodes[4].id, target: nodes[6].id, sourceHandle: 'port:err', targetHandle: 'port:data' },
    ]

    const laidOut = layoutComponentPipeline(nodes, edges)
    expect(laidOut).not.toBeNull()
    const byID = new Map(laidOut!.map((node) => [node.id, node]))
    const builder = byID.get(nodes[3].id)!
    const println = byID.get(nodes[4].id)!
    const stop = byID.get(nodes[5].id)!
    const panic = byID.get(nodes[6].id)!
    const age = byID.get(nodes[1].id)!
    const name = byID.get(nodes[2].id)!

    expect(age.position.x).toBeLessThan(name.position.x)
    expect(builder.position.y).toBeLessThan(println.position.y)
    expect(stop.position.x).toBeLessThan(panic.position.x)
    expect(stop.position.y).toBeGreaterThan(println.position.y)
    expect(panic.position.y).toBeGreaterThan(println.position.y)
  })
})

function portNode(id: string, portRole: 'in' | 'out', label: string): Node<NodeData> {
  return {
    id,
    type: 'entityNode',
    position: { x: 0, y: 0 },
    data: { kind: 'port', portRole, label, subtitle: 'any' },
  }
}

function constNode(id: string, label: string, subtitle: string): Node<NodeData> {
  return {
    id,
    type: 'entityNode',
    position: { x: 0, y: 0 },
    data: { kind: 'const', label, subtitle },
  }
}

function callNode(id: string, label: string, inPorts: NodeData['inPorts'], outPorts: NodeData['outPorts']): Node<NodeData> {
  return {
    id,
    type: 'entityNode',
    position: { x: 0, y: 0 },
    data: { kind: 'entity', label, inPorts, outPorts },
  }
}
