# Neva View web

Standalone UI for `neva/view/*` APIs.

Boundary contract:
- UI consumes only HTTP JSON endpoints under `/api/view/*`.
- UI does not import compiler/indexer/AST internals.
- The standalone backend is `neva-view`; `neva-lsp` consumes the same semantic view service through its JSON-RPC methods.

## Dev

```bash
npm install
npm run dev
```

## Build

```bash
npm run build
```

Build output is embedded by `neva-view` from `cmd/neva-view/assets`.
