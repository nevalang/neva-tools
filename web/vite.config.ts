import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => ({
  plugins: [react()],
  // The VS Code extension builds these exact sources into a WebView bundle.
  // Browser mode remains the embedded assets served by `neva-view`.
  base: mode === 'vscode' ? './' : '/',
  build: {
    outDir: mode === 'vscode' ? '../dist/webview' : '../cmd/neva-view/assets',
    emptyOutDir: true,
  },
}))
