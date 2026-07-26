# Neva Tools

This repository contains developer tools for the Neva language:

- `neva-lsp` provides diagnostics, navigation and language intelligence to editors.
- `neva-view` serves the standalone visual editor in a browser.

## Common commands

The [Makefile](Makefile) is the source of truth for local commands.

```bash
make install-lsp    # install only the language server; no Node.js required
make install-view   # build the React UI and install the standalone visual editor
make install-all    # install both commands
make test-all       # run Go and frontend tests
make view           # install and start neva-view for the current workspace
```

## Structure

```text
cmd/neva-lsp/       thin LSP executable entry point
cmd/neva-view/      standalone View CLI, HTTP host and embedded browser assets
internal/lsp/       LSP transport, lifecycle and language features
internal/viewservice/
                    shared ast.Build to visual-editor model and query layer
web/                React visual-editor source
```

`internal/viewservice` is shared by LSP JSON-RPC methods and the standalone View HTTP API. HTTP serving and browser launching are deliberately View-only and remain under `cmd/neva-view`.

## Release boundaries

One repository does not imply one release cycle. The independently consumable
components are released from component-prefixed tags:

- `neva-lsp/vX.Y.Z` publishes the six supported `neva-lsp` binaries and
  `SHA256SUMS`.
- `visual-editor/vX.Y.Z` publishes the VS Code WebView bundle and its
  `SHA256SUMS`.
- `neva-view` is a standalone CLI. It receives its own release only when it is
  distributed independently; it is not a dependency of VS Code.

The tags may initially point to the same source commit, but their versions are
independent contracts. Do not create a repository-wide `vX.Y.Z` release for
these components.
