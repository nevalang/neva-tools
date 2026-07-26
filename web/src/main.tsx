import { createHTTPViewBackend } from './hosts/httpViewBackend'
import { createVSCodeViewBackend } from './hosts/vscodeViewBackend'
import { mountApp } from './mountApp'

// This is a build-time host selection. The shared App and ViewBackend contract
// deliberately have no knowledge of VS Code or browser transport details.
mountApp(import.meta.env.MODE === 'vscode' ? createVSCodeViewBackend() : createHTTPViewBackend())
