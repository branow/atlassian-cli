#!/bin/sh
# Install the latest atl release binary.
#   curl -fsSL https://raw.githubusercontent.com/branow/atlassian-cli/main/scripts/install.sh | sh
# Environment:
#   ATL_INSTALL_DIR  target directory (default: /usr/local/bin, falls back
#                    to ~/.local/bin when /usr/local/bin is not writable)
#   ATL_VERSION      version to install, e.g. v0.1.0 (default: latest)
set -eu

REPO="branow/atlassian-cli"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux | darwin) ;;
  *) echo "error: unsupported OS: $os (use the Windows zip from GitHub releases)" >&2; exit 1 ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "error: unsupported architecture: $arch" >&2; exit 1 ;;
esac

version="${ATL_VERSION:-}"
if [ -z "$version" ]; then
  version=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    grep -m1 '"tag_name"' | cut -d '"' -f 4)
fi
[ -n "$version" ] || { echo "error: could not resolve the latest version" >&2; exit 1; }

archive="atl_${version#v}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$version/$archive"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $url"
curl -fsSL "$url" -o "$tmp/$archive"

curl -fsSL "https://github.com/$REPO/releases/download/$version/checksums.txt" -o "$tmp/checksums.txt"
(cd "$tmp" && grep " $archive\$" checksums.txt | sha256sum -c - >/dev/null 2>&1) ||
  (cd "$tmp" && grep " $archive\$" checksums.txt | shasum -a 256 -c - >/dev/null) ||
  { echo "error: checksum verification failed" >&2; exit 1; }

tar -xzf "$tmp/$archive" -C "$tmp" atl

dir="${ATL_INSTALL_DIR:-/usr/local/bin}"
if [ ! -w "$dir" ] && [ -z "${ATL_INSTALL_DIR:-}" ]; then
  dir="$HOME/.local/bin"
  mkdir -p "$dir"
fi

install -m 755 "$tmp/atl" "$dir/atl"
echo "Installed atl $version to $dir/atl"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "note: $dir is not on your PATH" ;;
esac
