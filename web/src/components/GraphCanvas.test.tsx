import { describe, expect, it } from 'vitest'
import type { Edge, Node } from '@xyflow/react'
import { catalogColumnCount, fileEntityNodes, fileNodes, layoutCatalogGrid, layoutComponentPipeline, nodeTypeArgs, packageNodes, shouldUseMeasuredLayoutSizes, type NodeData } from './GraphCanvas'
import type { FileView, ModuleSummary } from '../lib/types'

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
    expect(constNode?.style?.width).toBe(280)
    const typeNode = nodes.find((node) => node.data.label === 'AType')
    expect(typeNode?.data.subtitle).toBe('type · string')
    expect(typeNode?.style?.width).toBe(280)
  })

  it('uses measured DOM sizes only for entity graphs', () => {
    expect(shouldUseMeasuredLayoutSizes({ kind: 'modules' }, 0)).toBe(false)
    expect(shouldUseMeasuredLayoutSizes({ kind: 'module', modulePath: '@' }, 0)).toBe(false)
    expect(shouldUseMeasuredLayoutSizes({ kind: 'package', modulePath: '@', packageName: 'const_refs' }, 0)).toBe(false)
    expect(shouldUseMeasuredLayoutSizes({ kind: 'file', fileId: 'module/@/package/const_refs/file/main' }, 0)).toBe(false)
    expect(shouldUseMeasuredLayoutSizes({ kind: 'entity', fileId: 'module/@/package/const_refs/file/main', entityId: 'module/@/package/const_refs/file/main/component/Main@0' }, 0)).toBe(false)
    expect(shouldUseMeasuredLayoutSizes({ kind: 'entity', fileId: 'module/@/package/const_refs/file/main', entityId: 'module/@/package/const_refs/file/main/component/Main@0' }, 3)).toBe(true)
  })

  it('formats structured const values and lays file entities out without overlap', () => {
    const file: FileView = {
      id: 'module/@/package/const_refs/file/main',
      name: 'main',
      imports: [],
      components: [
        { id: 'c-main', name: 'Main', inPorts: [{ name: 'start', type: 'any' }], outPorts: [{ name: 'stop', type: 'any' }], nodes: [], connections: [] },
      ],
      interfaces: [],
      types: [{ id: 't-nums', name: 'NumsStruct', type: '<> = { d dict<int>, l list<int> }' }],
      consts: [
        { id: 'k-list', name: 'numsList', type: 'list<int>', value: '[one, two, three]' },
        { id: 'k-map', name: 'numsMap', type: 'dict<int>', value: '{"key": one}' },
        {
          id: 'k-struct',
          name: 'numsStruct',
          type: 'NumsStruct',
          value: '{"d": numsMap"l": numsList}',
          anchor: { text: 'numsStructNumsStruct={\nl:numsList,\nd:numsMap\n}\n' },
        },
        { id: 'k-one', name: 'one', type: 'int', value: '1' },
      ],
    }

    const nodes = fileEntityNodes(file)
    const numsStruct = nodes.find((node) => node.data.label === 'numsStruct')
    expect(numsStruct?.data.detail).toBe('{\n  l: numsList,\n  d: numsMap\n}')

    expect(catalogColumnCount({ kind: 'file', fileId: file.id }, nodes)).toBe(2)
    const laidOut = layoutCatalogGrid(nodes, 2)
    const first = laidOut.find((node) => node.data.label === 'Main')!
    const third = laidOut.find((node) => node.data.label === 'numsList')!
    expect(third.position.y).toBeGreaterThan(first.position.y)
    expect(third.position.y).toBeGreaterThanOrEqual(first.position.y + 150)
  })

  it('labels package and file catalog nodes without repeating package paths', () => {
    const modules: ModuleSummary[] = [{
      path: '@',
      packages: [{
        name: 'image_png',
        fileSummaries: [{ id: 'module/@/package/image_png/file/main', name: 'main' }],
      }],
    }]

    const packages = packageNodes(modules, '@')
    expect(packages[0].data.subtitle).toBe('package · 1 file')
    expect(packages[0].data.navType).toBe('package')

    const files = fileNodes(modules, '@', 'image_png')
    expect(files[0].data.label).toBe('main.neva')
    expect(files[0].data.subtitle).toBe('file')
  })

  it('allows four catalog columns on wide module and package views', () => {
    const previousWindow = globalThis.window
    Object.defineProperty(globalThis, 'window', {
      value: { innerWidth: 1600 },
      configurable: true,
    })

    const nodes: Node<NodeData>[] = Array.from({ length: 8 }, (_, index) => ({
      id: `package-${index}`,
      type: 'entityNode',
      position: { x: 0, y: 0 },
      data: { kind: 'nav', navType: 'package', label: `pkg_${index}`, subtitle: 'package · 1 file' },
    }))

    expect(catalogColumnCount({ kind: 'module', modulePath: '@' }, nodes)).toBe(4)
    Object.defineProperty(globalThis, 'window', {
      value: previousWindow,
      configurable: true,
    })
  })

  it('keeps multiple input port nodes separated when they target one call', () => {
    const nodes: Node<NodeData>[] = [
      portNode('component/NewPixel@0::in::c', 'in', 'c', 'image.RGBA'),
      portNode('component/NewPixel@0::in::x', 'in', 'x', 'int'),
      portNode('component/NewPixel@0::in::y', 'in', 'y', 'int'),
      callNode(
        'component/NewPixel@0::node::pb',
        'pb',
        [{ name: 'color', type: '' }, { name: 'x', type: '' }, { name: 'y', type: '' }],
        [{ name: 'resT', type: '' }],
      ),
      portNode('component/NewPixel@0::out::pixel', 'out', 'pixel', 'image.Pixel'),
    ]
    const edges: Edge[] = [
      { id: 'c-color', source: nodes[0].id, target: nodes[3].id, targetHandle: 'port:color' },
      { id: 'x-x', source: nodes[1].id, target: nodes[3].id, targetHandle: 'port:x' },
      { id: 'y-y', source: nodes[2].id, target: nodes[3].id, targetHandle: 'port:y' },
      { id: 'pb-pixel', source: nodes[3].id, target: nodes[4].id, sourceHandle: 'port:resT' },
    ]

    const laidOut = layoutComponentPipeline(nodes, edges)
    expect(laidOut).not.toBeNull()
    const inputPorts = laidOut!
      .filter((node) => node.id.includes('::in::'))
      .sort((a, b) => a.position.x - b.position.x)

    expect(inputPorts.map((node) => node.data.label)).toEqual(['c', 'x', 'y'])
    expect(inputPorts[1].position.x - inputPorts[0].position.x).toBeGreaterThanOrEqual(112)
    expect(inputPorts[2].position.x - inputPorts[1].position.x).toBeGreaterThanOrEqual(112)
  })

  it('prefers source-level type arguments for node display', () => {
    expect(nodeTypeArgs({
      anchor: { text: 'pbStruct<image.Pixel>' },
      entityRef: { name: 'Struct' },
      typeArgs: ['{ color { a int, b int, g int, r int }, x int, y int }'],
    })).toEqual(['image.Pixel'])

    expect(nodeTypeArgs({
      anchor: { text: 'pbStruct' },
      entityRef: { name: 'Struct' },
      typeArgs: ['{ x int }'],
    })).toEqual(['{ x int }'])
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
    expect(panic.position.x - stop.position.x).toBeGreaterThanOrEqual(112)
    expect(stop.position.y).toBeGreaterThan(println.position.y)
    expect(panic.position.y).toBeGreaterThan(println.position.y)
  })
})

function portNode(id: string, portRole: 'in' | 'out', label: string, subtitle = 'any'): Node<NodeData> {
  return {
    id,
    type: 'entityNode',
    position: { x: 0, y: 0 },
    data: { kind: 'port', portRole, label, subtitle },
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
