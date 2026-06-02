import type { FileSummary, FileView, ModuleSummary, Program } from './types'

export type AppRoute =
  | { kind: 'modules' }
  | { kind: 'module'; modulePath: string }
  | { kind: 'package'; modulePath: string; packageName: string }
  | { kind: 'file'; fileId: string }
  | { kind: 'entity'; fileId: string; entityId: string }

export function inferInitialRoute(programOrModules: Program | ModuleSummary[]): AppRoute {
  const modules = Array.isArray(programOrModules) ? programOrModules : programOrModules.modules
  const entryFileIds = Array.isArray(programOrModules) ? [] : (programOrModules.entryFileIds ?? [])
  if (modules.length === 0) {
    return { kind: 'modules' }
  }

  const activeModule = modules.find((item) => item.path === '@') ?? modules[0]
  if (!activeModule) {
    return { kind: 'modules' }
  }

  const entryFiles = entryFileIds
    .map((fileID) => findFileSummary(modules, fileID))
    .filter((file): file is FileSummary => Boolean(file))

  if (entryFiles.length === 1) {
    const route = singleComponentRoute(entryFiles[0])
    if (route) return route
    return { kind: 'file', fileId: entryFiles[0].id }
  }

  if (activeModule.packages.length === 1) {
    const onlyPackage = activeModule.packages[0]
    if (onlyPackage && onlyPackage.fileSummaries.length === 1) {
      const route = singleComponentRoute(onlyPackage.fileSummaries[0])
      if (route) return route
    }
  }

  return { kind: 'module', modulePath: activeModule.path }
}

function singleComponentRoute(file: FileSummary | undefined): AppRoute | null {
  if (!file) return null

  const componentEntities = file.components ?? []
  const interfaceEntities = file.interfaces ?? []
  const typeEntities = file.types ?? []
  const constEntities = file.consts ?? []
  const entityTotal =
    componentEntities.length +
    interfaceEntities.length +
    typeEntities.length +
    constEntities.length
  if (entityTotal === 1 && componentEntities.length === 1) {
    return { kind: 'entity', fileId: file.id, entityId: componentEntities[0].id }
  }
  return null
}

function findFileSummary(modules: ModuleSummary[], fileID: string): FileSummary | null {
  for (const moduleItem of modules) {
    for (const pkg of moduleItem.packages) {
      const file = pkg.fileSummaries.find((item) => item.id === fileID)
      if (file) return file
    }
  }
  return null
}

function fileSummaryHasEntity(file: FileSummary, entityID: string): boolean {
  const groups = [file.components, file.interfaces, file.types, file.consts]
  if (groups.every((group) => group === undefined)) {
    return true
  }
  return groups.some((group) => (group ?? []).some((entity) => entity.id === entityID))
}

export function routeExistsInProgram(route: AppRoute, modules: ModuleSummary[]): boolean {
  if (modules.length === 0) {
    return route.kind === 'modules'
  }

  if (route.kind === 'modules') {
    return true
  }

  if (route.kind === 'module') {
    return modules.some((moduleItem) => moduleItem.path === route.modulePath)
  }

  if (route.kind === 'package') {
    const moduleItem = modules.find((item) => item.path === route.modulePath)
    return Boolean(moduleItem?.packages.some((pkg) => pkg.name === route.packageName))
  }

  if (route.kind === 'file' || route.kind === 'entity') {
    const file = findFileSummary(modules, route.fileId)
    if (!file) return false
    if (route.kind === 'file') return true
    return fileSummaryHasEntity(file, route.entityId)
  }

  return false
}

export function isNativeComponent(file: FileView, entityID: string): boolean {
  const component = file.components.find((item) => item.id === entityID)
  if (!component) {
    return false
  }
  return component.nodes.length === 0 && component.connections.length === 0
}
