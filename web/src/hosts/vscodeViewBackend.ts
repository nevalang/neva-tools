import {
  normalizeFile,
  normalizeProgram,
  type ProgramFilters,
  type SearchFilters,
  type ViewBackend,
} from '../lib/viewClient'
import type { Program, ResolveEntityRefResult, SearchEntitiesResultItem } from '../lib/types'

type ViewMethod = 'neva/view/getProgram' | 'neva/view/getFileView' | 'neva/view/resolveEntityRef' | 'neva/view/searchEntities'
type VSCodeRequest = { type: 'neva/view/request'; id: string; method: ViewMethod; params: unknown }
type VSCodeResponse = { type: 'neva/view/response'; id: string; result?: unknown; error?: string }
type VSCodeAPI = { postMessage(message: VSCodeRequest): void }

declare global {
  interface Window { acquireVsCodeApi(): VSCodeAPI }
}

export function createVSCodeViewBackend(): ViewBackend {
  const vscode = window.acquireVsCodeApi()
  let sequence = 0
  const pending = new Map<string, { resolve(value: unknown): void; reject(reason: Error): void }>()

  window.addEventListener('message', (event: MessageEvent<VSCodeResponse>) => {
    const message = event.data
    if (message?.type !== 'neva/view/response') return
    const request = pending.get(message.id)
    if (!request) return
    pending.delete(message.id)
    if (message.error) request.reject(new Error(message.error))
    else request.resolve(message.result)
  })

  function request<T>(method: ViewMethod, params: unknown): Promise<T> {
    const id = `view-${Date.now()}-${sequence++}`
    return new Promise<T>((resolve, reject) => {
      pending.set(id, { resolve, reject })
      vscode.postMessage({ type: 'neva/view/request', id, method, params })
    })
  }

  return {
    async getProgram(filters: ProgramFilters): Promise<Program> {
      return normalizeProgram(await request<Program>('neva/view/getProgram', filters))
    },
    async getFileView(fileId) {
      return normalizeFile(await request('neva/view/getFileView', { fileId }))
    },
    searchEntities(filters: SearchFilters): Promise<SearchEntitiesResultItem[]> {
      return request('neva/view/searchEntities', {
        query: filters.query.trim(), kinds: filters.kinds,
        packageFilters: filters.packages, moduleFilters: filters.modules, limit: 100,
      })
    },
    resolveEntityRef(targetFileId, targetEntityId): Promise<ResolveEntityRefResult> {
      return request('neva/view/resolveEntityRef', { targetFileId, targetEntityId })
    },
    onRefresh(listener) {
      const receive = (event: MessageEvent<{ type?: string }>) => {
        if (event.data?.type === 'neva/view/refresh') listener()
      }
      window.addEventListener('message', receive)
      return () => window.removeEventListener('message', receive)
    },
  }
}
