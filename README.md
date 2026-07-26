# Neva Tools

Developer tools for the [Neva language](https://github.com/nevalang/neva): a Language Server Protocol implementation and a standalone visual-editor host.

## Commands

| Command | Purpose | Runtime dependencies |
| --- | --- | --- |
| `neva-lsp` | Language Server Protocol server for editors, using stdio by default | Go-built binary only |
| `neva-view` | Opens the visual editor in a browser and serves its HTTP API | Go-built binary with the React bundle embedded |

## Install

Install only the language server; this does not require Node.js:

```bash
make install
```

Install the standalone visual editor; this builds the browser bundle first:

```bash
make install-view
```

To install both commands:

```bash
make install-all
```

## Use

Editors start `neva-lsp` through stdio. For local TCP debugging:

```bash
neva-lsp --debug
```

Run the visual editor against a Neva workspace:

```bash
neva-view --port=7792 --workspace=/absolute/path/to/workspace
```

From the repository root:

```bash
make view
```

## Architecture

```text
cmd/neva-lsp  -> internal/lsp          -> internal/viewservice
cmd/neva-view -> HTTP/browser host     -> internal/viewservice
web/          -> bundled into cmd/neva-view/assets
```

`internal/lsp` owns the LSP transport, lifecycle and language features. `cmd/neva-view` owns HTTP, static files and browser launching. `internal/viewservice` is deliberately the only shared layer: it projects an analyzed Neva `ast.Build` into the visual-editor model and serves the same `program`, `file`, `search` and `resolve` queries to both tools.

The standalone `neva-view` host is not launched by VS Code. The extension should use the already-running LSP process and its `neva/view/*` JSON-RPC methods; this prevents a second workspace scan and keeps one source of semantic data.

## Development

```bash
make test-all
```

- `make test` runs Go tests.
- `make test-web` runs frontend unit tests with Vitest.
- `make web-build` refreshes the assets embedded into `neva-view`.
