import type { FileView, ModuleSummary } from './types'

export type AppRoute =
  | { kind: 'modules' }
  | { kind: 'module'; modulePath: string }
  | { kind: 'package'; modulePath: string; packageName: string }
  | { kind: 'file'; fileId: string }
  | { kind: 'entity'; fileId: string; entityId: string }

export function inferInitialRoute(modules: ModuleSummary[]): AppRoute {
  if (modules.length === 0) {
    return { kind: 'modules' }
  }

  const activeModule = modules.find((item) => item.path === '@') ?? modules[0]
  if (!activeModule) {
    return { kind: 'modules' }
  }

  return { kind: 'module', modulePath: activeModule.path }
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
    return modules.some((moduleItem) =>
      moduleItem.packages.some((pkg) =>
        pkg.fileSummaries.some((fileSummary) => fileSummary.id === route.fileId),
      ),
    )
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
