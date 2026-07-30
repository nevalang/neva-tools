import { afterEach, describe, expect, it, vi } from 'vitest'
import { createVSCodeViewBackend } from './vscodeViewBackend'

type Listener = (event: MessageEvent) => void

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('VS Code View backend', () => {
  it('retries getProgram while the LSP is building its initial index', async () => {
    vi.useFakeTimers()
    const listeners = new Set<Listener>()
    const requests: Array<{ id: string; method: string }> = []
    let attempt = 0
    const fakeWindow = {
      acquireVsCodeApi: () => ({
        postMessage: (request: { id: string; method: string }) => {
          requests.push(request)
          const response = attempt++ === 0
            ? { type: 'neva/view/response', id: request.id, error: 'program index is not ready' }
            : { type: 'neva/view/response', id: request.id, result: { modules: [], entryFileIds: [] } }
          queueMicrotask(() => listeners.forEach((listener) => listener({ data: response } as MessageEvent)))
        },
      }),
      addEventListener: (_type: string, listener: Listener) => listeners.add(listener),
      removeEventListener: (_type: string, listener: Listener) => listeners.delete(listener),
      setTimeout,
    }
    vi.stubGlobal('window', fakeWindow)

    const program = createVSCodeViewBackend().getProgram({
      includeCurrent: true,
      includeDeps: true,
      includeStd: true,
    })

    await vi.advanceTimersByTimeAsync(250)

    await expect(program).resolves.toEqual({ modules: [], entryFileIds: [] })
    expect(requests.map((request) => request.method)).toEqual([
      'neva/view/getProgram',
      'neva/view/getProgram',
    ])
  })

  it('does not retry non-transient view errors', async () => {
    const listeners = new Set<Listener>()
    const requests: Array<{ id: string; method: string }> = []
    const fakeWindow = {
      acquireVsCodeApi: () => ({
        postMessage: (request: { id: string; method: string }) => {
          requests.push(request)
          queueMicrotask(() => listeners.forEach((listener) => listener({
            data: { type: 'neva/view/response', id: request.id, error: 'workspace is invalid' },
          } as MessageEvent)))
        },
      }),
      addEventListener: (_type: string, listener: Listener) => listeners.add(listener),
      removeEventListener: (_type: string, listener: Listener) => listeners.delete(listener),
      setTimeout,
    }
    vi.stubGlobal('window', fakeWindow)

    await expect(createVSCodeViewBackend().getProgram({
      includeCurrent: true,
      includeDeps: true,
      includeStd: true,
    })).rejects.toThrow('workspace is invalid')
    expect(requests).toHaveLength(1)
  })
})
