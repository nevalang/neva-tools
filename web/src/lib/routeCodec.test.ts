import { describe, expect, it } from 'vitest'
import { parseHashRoute, routeToHash } from './routeCodec'

describe('routeCodec', () => {
  it('uses modules route for empty hash', () => {
    expect(parseHashRoute('')).toEqual({ kind: 'modules' })
    expect(parseHashRoute('#')).toEqual({ kind: 'modules' })
    expect(parseHashRoute('#/')).toEqual({ kind: 'modules' })
  })

  it('encodes current module as path segment', () => {
    expect(routeToHash({ kind: 'module', modulePath: '@' })).toBe('#/current')
    expect(parseHashRoute('#/current')).toEqual({ kind: 'module', modulePath: '@' })
  })

  it('roundtrips package and file routes', () => {
    expect(parseHashRoute('#/std/fmt')).toEqual({ kind: 'package', modulePath: 'std', packageName: 'fmt' })
    expect(parseHashRoute('#/std/fmt/main')).toEqual({ kind: 'file', fileId: 'module/std/package/fmt/file/main' })
  })

  it('supports module paths with slashes', () => {
    const hash = '#/github.com%2Fnevalang%2Fmodule/pkg/main'
    expect(parseHashRoute(hash)).toEqual({
      kind: 'file',
      fileId: 'module/github.com/nevalang/module/package/pkg/file/main',
    })
  })

  it('keeps entity route explicit as readable trailing segments', () => {
    const route = {
      kind: 'entity' as const,
      fileId: 'module/@/package/hello_world/file/main',
      entityId: 'module/@/package/hello_world/file/main/component/Main@0',
    }
    const hash = routeToHash(route)
    expect(hash).toBe('#/current/hello_world/main/component/Main~0')
    expect(parseHashRoute(hash)).toEqual(route)
  })
})
