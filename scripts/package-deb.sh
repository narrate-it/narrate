#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
  echo "usage: package-deb.sh VERSION ARCH BINARY OUTPUT" >&2
  exit 2
fi

version=$1
arch=$2
binary=$3
output=$4
case "$version" in ''|*[!0-9.]*) echo "invalid version: $version" >&2; exit 2 ;; esac
case "$arch" in amd64|arm64) ;; *) echo "unsupported Debian architecture: $arch" >&2; exit 2 ;; esac
[ -x "$binary" ] || { echo "binary is missing or not executable: $binary" >&2; exit 1; }
command -v dpkg-deb >/dev/null 2>&1 || { echo "dpkg-deb is required" >&2; exit 1; }

stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT HUP INT TERM
mkdir -p "$stage/DEBIAN" "$stage/usr/bin"
install -m 755 "$binary" "$stage/usr/bin/narrate"
cat > "$stage/DEBIAN/control" <<EOF
Package: narrate
Version: $version
Section: utils
Priority: optional
Architecture: $arch
Maintainer: Narrate maintainers <noreply@github.com>
Description: Conversational document narration CLI
 Narrate rewrites documents into spoken scripts and saves or plays speech.
EOF
mkdir -p "$(dirname "$output")"
dpkg-deb --root-owner-group --build "$stage" "$output"
