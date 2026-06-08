import { describe, expect, it } from 'vitest'
import { normalizeFile } from './viewClient'

describe('viewClient normalization', () => {
  it('preserves ordered ports and dependency injection args', () => {
    const file = normalizeFile({
      id: 'f',
      name: 'main',
      components: [{
        id: 'c',
        name: 'Main',
        inPorts: [
          { name: 'from', type: 'int', order: 0 },
          { name: 'to', type: 'int', order: 1 },
        ],
        outPorts: [],
        nodes: [{
          id: 'n',
          name: 'for_each',
          typeArgs: ['int'],
          diArgs: [{
            id: 'di',
            name: 'handler',
            nodeName: 'handler',
            entityRef: { name: 'Print2Lines' },
            resolvedRef: { canonicalRef: '@:/99_bottles/Print2Lines', fileId: 'f', entityId: 'p', entityKind: 'component_entity' },
          }],
        }],
        connections: [],
      }],
    })

    expect(file.components[0].inPorts.map((port) => port.name)).toEqual(['from', 'to'])
    expect(file.components[0].inPorts.map((port) => port.order)).toEqual([0, 1])
    expect(file.components[0].nodes[0].typeArgs).toEqual(['int'])
    expect(file.components[0].nodes[0].diArgs?.[0]).toMatchObject({
      name: 'handler',
      nodeName: 'handler',
      resolvedRef: { entityId: 'p' },
    })
  })

  it('preserves endpoint selectors on normalized connections', () => {
    const file = normalizeFile({
      id: 'f',
      name: 'main',
      components: [{
        id: 'c',
        name: 'Main',
        inPorts: [],
        outPorts: [],
        nodes: [],
        connections: [{
          id: 'edge-1',
          sender: { node: 'in', port: 'state', selector: ['rate'] },
          receiver: { node: 'mul_rate_balance', port: 'left' },
        }],
      }],
    })

    expect(file.components[0].connections[0].sender.selector).toEqual(['rate'])
  })
})
