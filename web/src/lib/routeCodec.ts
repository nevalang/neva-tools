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
  return modulePath.replace(/@/g, '--')
}

function decodeModulePath(raw: string): string {
  if (raw === 'current' || raw === 'user') return '@'
  return raw.replace(/--/g, '@')
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

function splitEntityNameAndOverload(entityName: string): { baseName: string; overload: string | null } {
  const match = entityName.match(/^(.*)@(\d+)$/u)
  if (!match) {
    return { baseName: entityName, overload: null }
  }
  return { baseName: match[1] ?? entityName, overload: match[2] ?? null }
}

function parseEntityID(entityID: string): { entityKind: string; entityName: string } {
  const parts = entityID.split('/')
  const len = parts.length
  return {
    entityKind: parts[len - 2] ?? '',
    entityName: parts[len - 1] ?? '',
  }
}

function composeEntityID(fileID: string, entityKind: string, entityName: string): string {
  return `${fileID}/${entityKind}/${entityName}`
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

  const entityKind = segments[3] ?? ''
  const entityName = segments[4] ?? ''
  if (!entityKind || !entityName) return { kind: 'file', fileId }
  const overload = segments[5] ?? ''
  const canonicalName = overload ? `${entityName}@${overload}` : entityName
  return { kind: 'entity', fileId, entityId: composeEntityID(fileId, entityKind, canonicalName) }
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
  const parsed = parseEntityID(route.entityId)
  const split = splitEntityNameAndOverload(parsed.entityName)
  const base = `#/${encodeSegment(encodeModulePath(parts.modulePath))}/${encodeSegment(parts.packageName)}/${encodeSegment(parts.fileName)}/${encodeSegment(parsed.entityKind)}/${encodeSegment(split.baseName)}`
  return split.overload ? `${base}/${encodeSegment(split.overload)}` : base
}
