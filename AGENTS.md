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
components use these exact tag and GitHub Release naming conventions:

- `lsp/vX.Y.Z` → `LSP vX.Y.Z`: six supported `neva-lsp` binaries and
  `SHA256SUMS`.
- `visual-editor/vX.Y.Z` → `Visual Editor vX.Y.Z`: VS Code WebView bundle and
  its `SHA256SUMS`.
- `view/vX.Y.Z` → `View vX.Y.Z`: six supported `neva-view` binaries with the
  browser UI embedded, plus `SHA256SUMS`. It is not a dependency of VS Code.

The tags may initially point to the same source commit, but their versions are
independent contracts. Do not create a repository-wide `vX.Y.Z` release for
these components.

### Stable and beta releases

- Stable: use `component/vX.Y.Z`; its GitHub Release is published normally.
- Beta: use `component/vX.Y.Z-beta.N` (for example `view/v0.2.0-beta.1`). The
  release workflow marks it as a GitHub prerelease automatically.
- Alpha and release-candidate tags use the same shape: `-alpha.N` and `-rc.N`.
- Create the tag only after its release workflow is merged into `main`; never
  move or reuse a published component tag.
