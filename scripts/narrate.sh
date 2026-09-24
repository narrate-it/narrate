#!/bin/sh
# Download verified releases and update the cached CLI when a new release is available.
set -eu
umask 077
fail() { echo "narrate: $*" >&2; exit 1; }
launcher=$0
while [ -L "$launcher" ]; do
  launcher_dir=$(CDPATH= cd -- "$(dirname -- "$launcher")" && pwd)
  target=$(readlink "$launcher")
  case "$target" in /*) launcher=$target ;; *) launcher=$launcher_dir/$target ;; esac
done
script_dir=$(CDPATH= cd -- "$(dirname -- "$launcher")" && pwd)
fallback_binary=${NARRATE_FALLBACK_BIN:-"$script_dir/narrate"}
pinned_version=$(cat "$script_dir/cli-version")
case "$pinned_version" in ''|*[!0-9.]*) fail 'Invalid bundled CLI version.' ;; esac
case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail 'Automatic installation supports macOS and Linux only.' ;;
esac
case "$(uname -m)" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64) arch=amd64 ;;
  *) fail 'Automatic installation supports ARM64 and x86-64 only.' ;;
esac
data_dir=${CLAUDE_PLUGIN_DATA:-${XDG_DATA_HOME:-$HOME/.local/share}/narrate-plugin}
case "$data_dir" in /*) ;; *) fail 'Plugin data directory must be an absolute path.' ;; esac
mkdir -p "$data_dir/cli"
check_file="$data_dir/cli/latest-$os-$arch"
now=$(date +%s)
release_tag=none
checked_at=0
if [ -f "$check_file" ]; then
  read -r release_tag checked_at < "$check_file" || true
fi
case "$release_tag" in none|v*) ;; *) release_tag=none ;; esac
case "$checked_at" in ''|*[!0-9]*) checked_at=0 ;; esac
if [ "$checked_at" -gt "$now" ] || [ $((now - checked_at)) -ge 21600 ]; then
  latest_url=$(curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --connect-timeout 5 --max-time 15 --output /dev/null --write-out '%{url_effective}' \
    https://github.com/narrate-it/narrate/releases/latest 2>/dev/null || true)
  case "$latest_url" in */releases/tag/v*) release_tag=${latest_url##*/} ;; *) release_tag=none ;; esac
  case "$release_tag" in
    v*)
      latest_version=${release_tag#v}
      case "$latest_version" in ''|*[!0-9.]*|.*|*.|*..*) release_tag=none ;; esac
      ;;
  esac
  stage=$(mktemp "$data_dir/cli/.latest.XXXXXX")
  printf '%s %s\n' "$release_tag" "$now" > "$stage"
  mv -f "$stage" "$check_file"
fi
version=$pinned_version
if [ "$release_tag" != none ]; then version=${release_tag#v}; fi
asset="narrate_${version}_${os}_${arch}"
cache_dir="$data_dir/cli/$version/$os-$arch"
binary="$cache_dir/narrate"
if [ "$release_tag" = none ] && [ -x "$fallback_binary" ]; then
  exec "$fallback_binary" "$@"
fi
if [ "$release_tag" != none ] && [ -x "$fallback_binary" ]; then
  fallback_version=$("$fallback_binary" --version 2>/dev/null | awk '{print $3}') || true
  if [ "$fallback_version" = "$version" ]; then exec "$fallback_binary" "$@"; fi
fi
if [ ! -x "$binary" ]; then
  command -v curl >/dev/null 2>&1 || fail 'Install curl to download Narrate.'
  if command -v sha256sum >/dev/null 2>&1; then
    hash_tool=sha256sum
  elif command -v shasum >/dev/null 2>&1; then
    hash_tool=shasum
  else
    fail 'Install sha256sum or shasum to verify Narrate.'
  fi
  mkdir -p "$cache_dir"
  stage=$(mktemp -d "$cache_dir/.download.XXXXXX")
  trap 'rm -rf "$stage"' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  base="https://github.com/narrate-it/narrate/releases/download/v$version"
  echo "narrate: Downloading v$version for $os/$arch..." >&2
  for file in "$asset" "$asset.sha256"; do
    if ! curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
      --connect-timeout 10 --max-time 120 --retry 2 --output "$stage/$file" "$base/$file"; then
      if [ -x "$fallback_binary" ]; then
        rm -rf "$stage"
        trap - EXIT INT TERM
        echo "narrate: update unavailable; using installed version" >&2
        exec "$fallback_binary" "$@"
      fi
      fail "Download failed. Check your connection and that release v$version is published; retry to resume setup."
    fi
  done
  expected=$(cat "$stage/$asset.sha256")
  case "$expected" in ''|*[!0-9a-f]*) fail 'Invalid release checksum.' ;; esac
  [ "${#expected}" -eq 64 ] || fail 'Invalid release checksum.'
  if [ "$hash_tool" = sha256sum ]; then
    actual=$(sha256sum "$stage/$asset")
  else
    actual=$(shasum -a 256 "$stage/$asset")
  fi
  actual=${actual%% *}
  [ "$actual" = "$expected" ] || fail 'Checksum mismatch; refusing to run downloaded binary.'
  chmod 700 "$stage/$asset"
  # Rename on the same filesystem: concurrent callers never see a partial binary.
  mv -f "$stage/$asset" "$binary"
  rm -rf "$stage"
  trap - EXIT INT TERM
fi
exec "$binary" "$@"
