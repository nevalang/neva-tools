# Neva Tools

Neva developer tools: `neva-lsp` and `neva-view`.

## Install Neva LSP

No Go installation is required to use the released language server.

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/nevalang/neva-tools/main/scripts/install-lsp.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/nevalang/neva-tools/main/scripts/install-lsp.ps1 | iex
```

The installers download the current platform binary from the latest `lsp/v*`
GitHub Release, verify it against `SHA256SUMS`, and install `neva-lsp` into a
user-writable directory on `PATH`. Pass `--version vX.Y.Z` to the shell
installer, or `-Version vX.Y.Z` to PowerShell, to install a specific release.

Use `neva-lsp version --json` to inspect the installed LSP component and the
Neva version it was built against.

See [AGENTS.md](AGENTS.md) for installation, commands, repository structure and contribution guidance.
