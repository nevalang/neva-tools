import type { AppRoute } from './appSemantics'

function encodeSegment(value: string): string {
  return encodeURIComponent(value)
}

function decodeSegment(value: string): string {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

function encodeModulePath(modulePath: string): string {
  if (modulePath === '@') return 'current'
  return modulePath
}

function decodeModulePath(raw: string): string {
  if (raw === 'current' || raw === 'user') return '@'
  return raw
}

function parseFileID(fileID: string): { modulePath: string; packageName: string; fileName: string } {
  const parts = fileID.split('/')
  const moduleIdx = parts.indexOf('module')
  const packageIdx = parts.indexOf('package')
  const fileIdx = parts.indexOf('file')
  return {
    modulePath: moduleIdx >= 0 ? (parts[moduleIdx + 1] ?? '') : '',
    packageName: packageIdx >= 0 ? (parts[packageIdx + 1] ?? '') : '',
    fileName: fileIdx >= 0 ? (parts[fileIdx + 1] ?? '') : '',
  }
}

function composeFileID(modulePath: string, packageName: string, fileName: string): string {
  return `module/${modulePath}/package/${packageName}/file/${fileName}`
}

export function parseHashRoute(hashRaw: string): AppRoute {
  const hash = hashRaw.replace(/^#/, '').replace(/^\/+/, '')
  if (!hash) return { kind: 'modules' }

  const segments = hash
    .split('/')
    .map((segment) => decodeSegment(segment))
    .filter((segment) => segment.length > 0)

  if (segments.length === 0) return { kind: 'modules' }

  const modulePath = decodeModulePath(segments[0] ?? '')
  if (!modulePath) return { kind: 'modules' }
  if (segments.length === 1) return { kind: 'module', modulePath }

  const packageName = segments[1] ?? ''
  if (!packageName) return { kind: 'module', modulePath }
  if (segments.length === 2) return { kind: 'package', modulePath, packageName }

  const fileName = segments[2] ?? ''
  if (!fileName) return { kind: 'package', modulePath, packageName }
  const fileId = composeFileID(modulePath, packageName, fileName)
  if (segments.length === 3) return { kind: 'file', fileId }

  const componentId = segments.slice(3).join('/')
  if (!componentId) return { kind: 'file', fileId }
  return { kind: 'component', fileId, componentId }
}

export function routeToHash(route: AppRoute): string {
  if (route.kind === 'modules') {
    return '#/'
  }

  if (route.kind === 'module') {
    return `#/${encodeSegment(encodeModulePath(route.modulePath))}`
  }

  if (route.kind === 'package') {
    return `#/${encodeSegment(encodeModulePath(route.modulePath))}/${encodeSegment(route.packageName)}`
  }

  if (route.kind === 'file') {
    const parts = parseFileID(route.fileId)
    return `#/${encodeSegment(encodeModulePath(parts.modulePath))}/${encodeSegment(parts.packageName)}/${encodeSegment(parts.fileName)}`
  }

  const parts = parseFileID(route.fileId)
  return `#/${encodeSegment(encodeModulePath(parts.modulePath))}/${encodeSegment(parts.packageName)}/${encodeSegment(parts.fileName)}/${encodeSegment(route.componentId)}`
}

