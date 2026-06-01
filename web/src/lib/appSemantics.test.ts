import { describe, expect, it } from 'vitest'
import { inferInitialRoute, isNativeComponent, routeExistsInProgram } from './appSemantics'
import type { FileView, ModuleSummary } from './types'

const modules: ModuleSummary[] = [
  { path: '@', packages: [{ name: 'main', fileSummaries: [{ id: 'module/@/package/main/file/main', name: 'main' }] }] },
  { path: 'std', packages: [] },
]

describe('appSemantics', () => {
  it('infers initial route from active module', () => {
    expect(inferInitialRoute(modules)).toEqual({ kind: 'module', modulePath: '@' })
  })

  it('auto-opens single-file single-component entrypoint', () => {
    const single: ModuleSummary[] = [
      {
        path: '@',
        packages: [{
          name: 'hello_world',
          fileSummaries: [{
            id: 'module/@/package/hello_world/file/main',
            name: 'main',
            components: [{ id: 'module/@/package/hello_world/file/main/component/Main@0', name: 'Main' }],
            interfaces: [],
            types: [],
            consts: [],
          }],
        }],
      },
    ]
    expect(inferInitialRoute(single)).toEqual({
      kind: 'entity',
      fileId: 'module/@/package/hello_world/file/main',
      entityId: 'module/@/package/hello_world/file/main/component/Main@0',
    })
  })

  it('validates routes against program', () => {
    expect(routeExistsInProgram({ kind: 'modules' }, modules)).toBe(true)
    expect(routeExistsInProgram({ kind: 'module', modulePath: '@' }, modules)).toBe(true)
    expect(routeExistsInProgram({ kind: 'package', modulePath: '@', packageName: 'main' }, modules)).toBe(true)
    expect(routeExistsInProgram({ kind: 'file', fileId: 'module/@/package/main/file/main' }, modules)).toBe(true)
    expect(routeExistsInProgram({ kind: 'module', modulePath: 'missing' }, modules)).toBe(false)
  })

  it('detects native-only components', () => {
    const file: FileView = {
      id: 'f',
      name: 'main',
      imports: [],
      interfaces: [],
      types: [],
      consts: [],
      components: [
        { id: 'c1', name: 'Native', inPorts: [], outPorts: [], nodes: [], connections: [] },
        { id: 'c2', name: 'Graph', inPorts: [], outPorts: [], nodes: [{ id: 'n', name: 'x' }], connections: [] },
      ],
    }
    expect(isNativeComponent(file, 'c1')).toBe(true)
    expect(isNativeComponent(file, 'c2')).toBe(false)
    expect(isNativeComponent(file, 'missing')).toBe(false)
  })
})
