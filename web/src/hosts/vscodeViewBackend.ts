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

const programIndexRetryDelayMs = 250
const programIndexRetryAttempts = 20

function isProgramIndexPending(error: unknown): boolean {
  return error instanceof Error && /program index is not ready/i.test(error.message)
}

function wait(delayMs: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, delayMs))
}

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

  async function requestProgram(filters: ProgramFilters): Promise<Program> {
    for (let attempt = 0; attempt < programIndexRetryAttempts; attempt += 1) {
      try {
        return await request<Program>('neva/view/getProgram', filters)
      } catch (error) {
        if (!isProgramIndexPending(error) || attempt === programIndexRetryAttempts - 1) {
          throw error
        }
        await wait(programIndexRetryDelayMs)
      }
    }

    throw new Error('program index did not become ready')
  }

  return {
    async getProgram(filters: ProgramFilters): Promise<Program> {
      // The LSP accepts requests before its initial workspace index is ready.
      // Treat that transient state as loading rather than leaving Visual Mode
      // permanently empty after its first request.
      return normalizeProgram(await requestProgram(filters))
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
