export type ViewMethod =
  | 'neva/view/getProgram'
  | 'neva/view/getFileView'
  | 'neva/view/resolveEntityRef'
  | 'neva/view/searchEntities'

type VSCodeRequest = {
  type: 'neva/view/request'
  id: string
  method: ViewMethod
  params: unknown
}

type VSCodeResponse = {
  type: 'neva/view/response'
  id: string
  result?: unknown
  error?: string
}

type VSCodeAPI = { postMessage(message: VSCodeRequest): void }

declare global {
  interface Window {
    acquireVsCodeApi?: () => VSCodeAPI
  }
}

let sequence = 0
const pending = new Map<string, { resolve(value: unknown): void; reject(reason: Error): void }>()
const vscode = typeof window !== 'undefined' ? window.acquireVsCodeApi?.() : undefined

if (vscode) {
  window.addEventListener('message', (event: MessageEvent<VSCodeResponse>) => {
    const message = event.data
    if (message?.type !== 'neva/view/response') return
    const request = pending.get(message.id)
    if (!request) return
    pending.delete(message.id)
    if (message.error) {
      request.reject(new Error(message.error))
    } else {
      request.resolve(message.result)
    }
  })
}

export function usesVSCodeTransport(): boolean {
  return Boolean(vscode)
}

export function requestVSCodeView<T>(method: ViewMethod, params: unknown): Promise<T> {
  if (!vscode) {
    return Promise.reject(new Error('VS Code visual-editor transport is unavailable'))
  }
  const id = `view-${Date.now()}-${sequence++}`
  return new Promise<T>((resolve, reject) => {
    pending.set(id, { resolve, reject })
    vscode.postMessage({ type: 'neva/view/request', id, method, params })
  })
}
