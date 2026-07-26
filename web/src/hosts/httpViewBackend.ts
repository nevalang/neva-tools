import {
  normalizeFile,
  normalizeProgram,
  type ProgramFilters,
  type SearchFilters,
  type ViewBackend,
} from '../lib/viewClient'
import type { Program, ResolveEntityRefResult, SearchEntitiesResultItem } from '../lib/types'

async function getJSON<T>(url: string): Promise<T> {
  const response = await fetch(url)
  if (!response.ok) throw new Error(await response.text())
  return (await response.json()) as T
}

async function postJSON<TReq extends object, TRes>(url: string, payload: TReq): Promise<TRes> {
  const response = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!response.ok) throw new Error(await response.text())
  return (await response.json()) as TRes
}

export function createHTTPViewBackend(): ViewBackend {
  return {
    async getProgram(filters: ProgramFilters): Promise<Program> {
      const query = new URLSearchParams({
        includeCurrent: String(filters.includeCurrent),
        includeDeps: String(filters.includeDeps),
        includeStd: String(filters.includeStd),
      })
      return normalizeProgram(await getJSON<Program>(`/api/view/program?${query.toString()}`))
    },
    async getFileView(fileId) {
      return normalizeFile(await getJSON(`/api/view/file?id=${encodeURIComponent(fileId)}`))
    },
    searchEntities(filters: SearchFilters): Promise<SearchEntitiesResultItem[]> {
      const query = new URLSearchParams({ q: filters.query.trim() })
      for (const kind of filters.kinds) query.append('kind', kind)
      for (const pkg of filters.packages) query.append('package', pkg)
      for (const modulePath of filters.modules) query.append('module', modulePath)
      return getJSON<SearchEntitiesResultItem[]>(`/api/view/search?${query.toString()}`)
    },
    resolveEntityRef(targetFileId, targetEntityId): Promise<ResolveEntityRefResult> {
      return postJSON('/api/view/resolve', { targetFileId, targetEntityId })
    },
    onRefresh: () => () => {},
  }
}
