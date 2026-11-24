#!/usr/bin/env sh
# Installs the sigil CLI from GitHub releases and verifies its checksum.
set -eu

REPO="${SIGIL_REPO:-ryan-tobin/new-startup}"
VERSION="${SIGIL_VERSION:-latest}"
BIN_DIR="${SIGIL_BIN_DIR:-/usr/local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "sigil: unsupported architecture $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux | darwin) ;;
  *) echo "sigil: unsupported operating system $os" >&2; exit 1 ;;
esac

if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -n1)
fi
VERSION=${VERSION#v}
[ -n "$VERSION" ] || { echo "sigil: could not determine version" >&2; exit 1; }

archive="sigil_${VERSION}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/v$VERSION"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

curl -fsSL -o "$tmp/$archive" "$base/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"

expected=$(grep " $archive\$" "$tmp/checksums.txt" | awk '{print $1}')
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
fi
[ "$expected" = "$actual" ] || { echo "sigil: checksum mismatch for $archive" >&2; exit 1; }

tar -xzf "$tmp/$archive" -C "$tmp" sigil
if [ -w "$BIN_DIR" ]; then
  install -m 0755 "$tmp/sigil" "$BIN_DIR/sigil"
else
  sudo install -m 0755 "$tmp/sigil" "$BIN_DIR/sigil"
fi
echo "sigil $VERSION installed to $BIN_DIR/sigil"
