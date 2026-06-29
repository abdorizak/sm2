#!/usr/bin/env bash
#
# sm2 installer.
#
#   curl -fsSL https://raw.githubusercontent.com/abdorizak/sm2/main/install.sh | bash
#
# Env overrides:
#   SM2_VERSION=v0.1.0-dev.3     install a specific release (default: latest)
#   SM2_INSTALL_DIR=/usr/local/bin   where to put the binary
#
set -euo pipefail

REPO="abdorizak/sm2"
BINARY="sm2"

info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
err()  { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || err "curl is required"
command -v tar  >/dev/null 2>&1 || err "tar is required"

# --- detect OS / architecture ---
os=$(uname -s)
arch=$(uname -m)
case "$os" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) err "unsupported OS: $os (sm2 supports Linux and macOS)" ;;
esac
case "$arch" in
  x86_64|amd64)  arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) err "unsupported architecture: $arch" ;;
esac

# --- resolve version (latest release, including pre-releases) ---
version="${SM2_VERSION:-}"
if [ -z "$version" ]; then
  info "Resolving latest release…"
  # Fetch fully, then parse — piping curl into `grep -m1` closes the pipe early
  # and trips pipefail with "curl: (23)".
  releases=$(curl -fsSL "https://api.github.com/repos/$REPO/releases") \
    || err "could not query GitHub releases (set SM2_VERSION=… to skip)"
  version=$(printf '%s\n' "$releases" | awk -F'"' '/"tag_name":/ && v==""{v=$4} END{print v}')
fi
[ -n "$version" ] || err "could not determine latest version (set SM2_VERSION=…)"

asset="${BINARY}_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$version"

# --- download ---
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
info "Downloading $asset ($version)"
curl -fsSL -o "$tmp/$asset" "$base/$asset" || err "download failed: $base/$asset"

# --- verify checksum (best-effort) ---
if curl -fsSL -o "$tmp/SHA256SUMS" "$base/SHA256SUMS" 2>/dev/null; then
  info "Verifying checksum"
  ( cd "$tmp"
    line=$(grep -F "$asset" SHA256SUMS || true)
    [ -n "$line" ] || err "no checksum for $asset"
    if command -v sha256sum >/dev/null 2>&1; then
      printf '%s\n' "$line" | sha256sum -c - >/dev/null 2>&1 || err "checksum mismatch"
    elif command -v shasum >/dev/null 2>&1; then
      printf '%s\n' "$line" | shasum -a 256 -c - >/dev/null 2>&1 || err "checksum mismatch"
    fi
  )
fi

tar -xzf "$tmp/$asset" -C "$tmp" "$BINARY"

# --- install ---
dir="${SM2_INSTALL_DIR:-/usr/local/bin}"
if mkdir -p "$dir" 2>/dev/null && [ -w "$dir" ]; then
  install -m 0755 "$tmp/$BINARY" "$dir/$BINARY"
elif command -v sudo >/dev/null 2>&1; then
  info "Installing to $dir (sudo)"
  sudo mkdir -p "$dir"
  sudo install -m 0755 "$tmp/$BINARY" "$dir/$BINARY"
else
  dir="$HOME/.local/bin"
  mkdir -p "$dir"
  install -m 0755 "$tmp/$BINARY" "$dir/$BINARY"
  info "Installed to $dir — add it to your PATH:"
  printf '    export PATH="%s:$PATH"\n' "$dir"
fi

info "Installed sm2 $version → $dir/$BINARY"
"$dir/$BINARY" version 2>/dev/null || true
cat <<'EOF'

Get started:
  sm2 start web --cmd "python3 -m http.server 8080" --restart always
  sm2 status
  sm2 --help

Docs: https://sm2.dev
EOF
