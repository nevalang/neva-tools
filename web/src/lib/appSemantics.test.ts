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

  it('uses standalone entry file without hiding other current packages', () => {
    const program = {
      entryFileIds: ['module/@/package/hello_world/file/main'],
      modules: [
        {
          path: '@',
          packages: [
            {
              name: 'hello_world',
              fileSummaries: [{
                id: 'module/@/package/hello_world/file/main',
                name: 'main',
                components: [{ id: 'module/@/package/hello_world/file/main/component/Main@0', name: 'Main' }],
                interfaces: [],
                types: [],
                consts: [],
              }],
            },
            {
              name: 'other',
              fileSummaries: [{ id: 'module/@/package/other/file/main', name: 'main' }],
            },
          ],
        },
      ],
    }

    expect(inferInitialRoute(program)).toEqual({
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

  it('validates entity routes against file summaries when entity refs are present', () => {
    const richModules: ModuleSummary[] = [{
      path: 'std@0.38.0',
      packages: [{
        name: 'http',
        fileSummaries: [{
          id: 'module/std@0.38.0/package/http/file/http',
          name: 'http',
          components: [{ id: 'module/std@0.38.0/package/http/file/http/component/Get@0', name: 'Get' }],
          interfaces: [],
          types: [{ id: 'module/std@0.38.0/package/http/file/http/type/Response', name: 'Response' }],
          consts: [],
        }],
      }],
    }]

    expect(routeExistsInProgram({
      kind: 'entity',
      fileId: 'module/std@0.38.0/package/http/file/http',
      entityId: 'module/std@0.38.0/package/http/file/http/type/Response',
    }, richModules)).toBe(true)
    expect(routeExistsInProgram({
      kind: 'entity',
      fileId: 'module/std@0.38.0/package/http/file/http',
      entityId: 'module/std@0.38.0/package/http/file/http/type/Missing',
    }, richModules)).toBe(false)
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
