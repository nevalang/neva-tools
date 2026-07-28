#!/usr/bin/env sh
set -eu

repository="nevalang/neva-tools"
install_dir="${NEVA_LSP_INSTALL_DIR:-$HOME/.local/bin}"
requested_version=""

usage() {
  cat <<'EOF'
Install the Neva Language Server from an official GitHub Release.

Usage: install-lsp.sh [--version vX.Y.Z] [--install-dir PATH]

Environment:
  NEVA_LSP_INSTALL_DIR  Default installation directory (default: ~/.local/bin)
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      requested_version="${2:?--version needs a value}"
      shift 2
      ;;
    --install-dir)
      install_dir="${2:?--install-dir needs a value}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "$(uname -s)" in
  Darwin) platform="darwin" ;;
  Linux) platform="linux" ;;
  *) echo "Unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64) architecture="amd64" ;;
  arm64|aarch64) architecture="arm64" ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

if [ -n "$requested_version" ]; then
  requested_version="${requested_version#lsp/}"
  requested_version="${requested_version#v}"
  release_tag="lsp/v${requested_version}"
else
  release_tag="$(curl --fail --silent --show-error --location \
    "https://api.github.com/repos/${repository}/releases?per_page=100" \
    | sed -n 's/^[[:space:]]*"tag_name": "\(lsp\/v[^"]*\)".*/\1/p' \
    | head -n 1)"
fi

if [ -z "$release_tag" ]; then
  echo "Could not find an LSP component release in ${repository}" >&2
  exit 1
fi

asset="neva-lsp-${platform}-${architecture}"
archive="https://github.com/${repository}/releases/download/${release_tag}"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT INT TERM

echo "Installing Neva LSP ${release_tag#lsp/} for ${platform}/${architecture}..."
curl --fail --silent --show-error --location "${archive}/SHA256SUMS" -o "${temporary_dir}/SHA256SUMS"
curl --fail --silent --show-error --location "${archive}/${asset}" -o "${temporary_dir}/${asset}"

expected_checksum="$(awk -v asset="$asset" '$2 == asset { print $1 }' "${temporary_dir}/SHA256SUMS")"
if [ -z "$expected_checksum" ]; then
  echo "Release checksum is missing for ${asset}" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual_checksum="$(sha256sum "${temporary_dir}/${asset}" | awk '{ print $1 }')"
else
  actual_checksum="$(shasum -a 256 "${temporary_dir}/${asset}" | awk '{ print $1 }')"
fi
if [ "$actual_checksum" != "$expected_checksum" ]; then
  echo "Checksum verification failed for ${asset}" >&2
  exit 1
fi

mkdir -p "$install_dir"
install -m 755 "${temporary_dir}/${asset}" "${install_dir}/neva-lsp"
echo "Installed neva-lsp to ${install_dir}/neva-lsp"

case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *)
    echo "Add ${install_dir} to PATH, then open a new terminal before running neva tool lsp."
    ;;
esac
