import { useEffect, useMemo, useState } from 'react'
import { GraphCanvas } from './components/GraphCanvas'
import { inferInitialRoute, routeExistsInProgram, type AppRoute } from './lib/appSemantics'
import { parseHashRoute, routeToHash } from './lib/routeCodec'
import { viewClient } from './lib/viewClient'
import type { FileView, ModuleSummary, Program } from './lib/types'

type Route = AppRoute

type Breadcrumb = {
  key: string
  label: string
  route: Route
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

function displayModuleLabel(modulePath: string): string {
  if (modulePath === '@') {
    return modulePath
  }
  return modulePath.replace(/@(?:v?\d[\w.-]*)$/, '')
}

function displayEntityName(rawID: string): string {
  const last = rawID.split('/').pop() ?? rawID
  return last.replace(/@\d+$/, '')
}

function routeKey(route: Route): string {
  return JSON.stringify(route)
}

function routesEqual(a: Route, b: Route): boolean {
  return routeKey(a) === routeKey(b)
}

function routeFileID(route: Route): string | null {
  if (route.kind === 'file' || route.kind === 'entity') {
    return route.fileId
  }
  return null
}

function fileDisplayName(fileID: string): string {
  const { fileName } = parseFileID(fileID)
  if (fileName) {
    return `${fileName}.neva`
  }

  return fileID
}


export function App() {
  const [modules, setModules] = useState<ModuleSummary[]>([])
  const [programMeta, setProgramMeta] = useState<Pick<Program, 'entryFileIds'>>({})
  const [route, setRoute] = useState<Route>(() => parseHashRoute(window.location.hash))
  const [fileCache, setFileCache] = useState<Record<string, FileView>>({})
  const [backStack, setBackStack] = useState<Route[]>([])
  const [forwardStack, setForwardStack] = useState<Route[]>([])

  useEffect(() => {
    void reloadProgram()
  }, [])

  useEffect(() => {
    function onRefresh(event: MessageEvent<{ type?: string }>) {
      if (event.data?.type !== 'neva/view/refresh') return
      setFileCache({})
      void reloadProgram()
    }

    window.addEventListener('message', onRefresh)
    return () => window.removeEventListener('message', onRefresh)
  }, [])

  useEffect(() => {
    const fileID = routeFileID(route)
    if (!fileID || fileCache[fileID]) {
      return
    }

    void viewClient.getFileView(fileID)
      .then((file) => {
        setFileCache((prev) => ({ ...prev, [fileID]: file }))
      })
      .catch(() => {
        const fallback = inferInitialRoute({ modules, entryFileIds: programMeta.entryFileIds })
        setRoute(fallback)
        window.history.replaceState({}, '', routeToHash(fallback))
      })
  }, [route, fileCache, modules, programMeta.entryFileIds])

  useEffect(() => {
    if (modules.length === 0) {
      return
    }
    if (routeExistsInProgram(route, modules)) {
      return
    }
    const fallback = inferInitialRoute({ modules, entryFileIds: programMeta.entryFileIds })
    setRoute(fallback)
    window.history.replaceState({}, '', routeToHash(fallback))
  }, [route, modules, programMeta.entryFileIds])

  useEffect(() => {
    function onPopState() {
      setRoute(parseHashRoute(window.location.hash))
    }

    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  async function reloadProgram() {
    const program = await viewClient.getProgram({
      includeCurrent: true,
      includeDeps: true,
      includeStd: true,
    })
    setModules(program.modules)
    setProgramMeta({ entryFileIds: program.entryFileIds })

    const initialRoute = inferInitialRoute(program)
    const hasExplicitHash = window.location.hash.replace(/^#/, '').length > 0
    setRoute((current) => {
      if (hasExplicitHash && routeExistsInProgram(current, program.modules)) {
        return current
      }
      if (routeKey(initialRoute) === routeKey(current)) {
        return current
      }
      window.history.replaceState({}, '', routeToHash(initialRoute))
      return initialRoute
    })
  }

  function navigate(next: Route, trackNav = true) {
    if (routesEqual(route, next)) {
      return
    }

    if (trackNav) {
      setBackStack((prev) => [...prev, route])
      setForwardStack([])
    }

    setRoute(next)
    window.history.pushState({}, '', routeToHash(next))
  }

  function goBack() {
    const prev = backStack[backStack.length - 1]
    if (!prev) {
      return
    }

    setBackStack((items) => items.slice(0, -1))
    setForwardStack((items) => [...items, route])
    setRoute(prev)
    window.history.replaceState({}, '', routeToHash(prev))
  }

  function goForward() {
    const next = forwardStack[forwardStack.length - 1]
    if (!next) {
      return
    }

    setForwardStack((items) => items.slice(0, -1))
    setBackStack((items) => [...items, route])
    setRoute(next)
    window.history.replaceState({}, '', routeToHash(next))
  }

  const selectedFile = useMemo(() => {
    const fileID = routeFileID(route)
    if (!fileID) {
      return null
    }
    return fileCache[fileID] ?? null
  }, [route, fileCache])

  const breadcrumbs = useMemo<Breadcrumb[]>(() => {
    if (route.kind === 'modules') {
      return [{ key: 'modules', label: 'modules', route: { kind: 'modules' } }]
    }

    if (route.kind === 'module') {
      return [
        { key: 'modules', label: 'modules', route: { kind: 'modules' } },
        { key: `module:${route.modulePath}`, label: displayModuleLabel(route.modulePath), route },
      ]
    }

    if (route.kind === 'package') {
      return [
        { key: 'modules', label: 'modules', route: { kind: 'modules' } },
        { key: `module:${route.modulePath}`, label: displayModuleLabel(route.modulePath), route: { kind: 'module', modulePath: route.modulePath } },
        { key: `package:${route.modulePath}:${route.packageName}`, label: route.packageName, route },
      ]
    }

    if (route.kind === 'file') {
      const fileID = route.fileId
      const { modulePath, packageName } = parseFileID(fileID)
      return [
        { key: 'modules', label: 'modules', route: { kind: 'modules' } },
        { key: `module:${modulePath}`, label: displayModuleLabel(modulePath), route: { kind: 'module', modulePath } },
        { key: `package:${modulePath}:${packageName}`, label: packageName, route: { kind: 'package', modulePath, packageName } },
        { key: `file:${fileID}`, label: fileDisplayName(fileID), route },
      ]
    }

    const fileID = route.fileId
    const { modulePath, packageName } = parseFileID(fileID)
    const entityName = displayEntityName(route.entityId)

    return [
      { key: 'modules', label: 'modules', route: { kind: 'modules' } },
      { key: `module:${modulePath}`, label: displayModuleLabel(modulePath), route: { kind: 'module', modulePath } },
      { key: `package:${modulePath}:${packageName}`, label: packageName, route: { kind: 'package', modulePath, packageName } },
      { key: `file:${fileID}`, label: fileDisplayName(fileID), route: { kind: 'file', fileId: fileID } },
      { key: `entity:${route.entityId}`, label: entityName, route },
    ]
  }, [route])

  async function resolveAndOpen(target: { fileId: string; entityId: string }) {
    const result = await viewClient.resolveEntityRef(target.fileId, target.entityId)
    const nextRoute: Route = { kind: 'entity', fileId: result.targetFileId, entityId: result.targetEntityId }
    navigate(nextRoute, true)
  }

  return (
    <main className="single-canvas-layout">
      <GraphCanvas
        modules={modules}
        route={route}
        file={selectedFile}
        breadcrumbs={breadcrumbs}
        canGoBack={backStack.length > 0}
        canGoForward={forwardStack.length > 0}
        onGoBack={goBack}
        onGoForward={goForward}
        onNavigate={navigate}
        onResolveOpen={resolveAndOpen}
      />
    </main>
  )
}
