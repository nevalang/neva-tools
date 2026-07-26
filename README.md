# Neva Tools

Language server for the Neva programming language.

## Install

```bash
make install
```

`make install` installs `neva-lsp`. `make install-view` builds the shared React bundle and installs the separate `neva-view` CLI.

## Tests

```bash
make test-all
```

- `make test` runs Go tests.
- `make test-web` runs frontend unit tests (`vitest`).

## Standalone view

Run from any directory and point to a Neva workspace explicitly:

```bash
neva-view --port=7792 --workspace=/absolute/path/to/workspace
```

If you are already inside the workspace:

```bash
make view
```

`neva-view` is intentionally separate from `neva-lsp`: it owns HTTP and browser hosting, while both tools use the same transport-neutral `internal/viewservice` for visual-editor queries. Build the UI before installation:

```bash
cd web
npm ci
npm run build
```
