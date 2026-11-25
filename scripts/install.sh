#!/usr/bin/env sh
# Installs the seralith CLI from GitHub releases and verifies its checksum.
set -eu

REPO="${SERALITH_REPO:-ryan-tobin/new-startup}"
VERSION="${SERALITH_VERSION:-latest}"
BIN_DIR="${SERALITH_BIN_DIR:-/usr/local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "seralith: unsupported architecture $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux | darwin) ;;
  *) echo "seralith: unsupported operating system $os" >&2; exit 1 ;;
esac

if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -n1)
fi
VERSION=${VERSION#v}
[ -n "$VERSION" ] || { echo "seralith: could not determine version" >&2; exit 1; }

archive="seralith_${VERSION}_${os}_${arch}.tar.gz"
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
[ "$expected" = "$actual" ] || { echo "seralith: checksum mismatch for $archive" >&2; exit 1; }

tar -xzf "$tmp/$archive" -C "$tmp" seralith
if [ -w "$BIN_DIR" ]; then
  install -m 0755 "$tmp/seralith" "$BIN_DIR/seralith"
else
  sudo install -m 0755 "$tmp/seralith" "$BIN_DIR/seralith"
fi
echo "seralith $VERSION installed to $BIN_DIR/seralith"
