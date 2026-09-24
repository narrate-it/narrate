#!/bin/sh
set -eu

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
  echo "usage: update-homebrew-formula.sh VERSION DIST_DIR [OUTPUT]" >&2
  exit 2
fi

version=$1
dist=$2
case "$version" in ''|*[!0-9.]*) echo "invalid version: $version" >&2; exit 2 ;; esac
checksum() {
  file=$dist/narrate_${version}_$1_$2
  [ -f "$file" ] && [ -f "$file.sha256" ] || {
    echo "missing release binary or checksum: $file" >&2
    exit 1
  }
  read -r digest _ < "$file.sha256"
  case "$digest" in
    *[!0-9a-f]*|'') echo "invalid SHA-256 checksum for $file" >&2; exit 1 ;;
  esac
  [ "${#digest}" -eq 64 ] || { echo "invalid SHA-256 checksum for $file" >&2; exit 1; }
  printf '%s' "$digest"
}

mac_arm=$(checksum darwin arm64)
mac_intel=$(checksum darwin amd64)
linux_arm=$(checksum linux arm64)
linux_intel=$(checksum linux amd64)
output=${3:-Formula/narrate.rb}
mkdir -p "$(dirname "$output")"
cat > "$output" <<EOF
class Narrate < Formula
  desc "Conversational document narration CLI"
  homepage "https://github.com/narrate-it/narrate"
  version "$version"

  on_macos do
    on_arm do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_darwin_arm64"
      sha256 "$mac_arm"
    end
    on_intel do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_darwin_amd64"
      sha256 "$mac_intel"
    end
  end
  on_linux do
    on_arm do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_linux_arm64"
      sha256 "$linux_arm"
    end
    on_intel do
      url "https://github.com/narrate-it/narrate/releases/download/v#{version}/narrate_#{version}_linux_amd64"
      sha256 "$linux_intel"
    end
  end

  def install
    bin.install cached_download => "narrate"
  end

  test do
    assert_match "narrate version #{version}", shell_output("#{bin}/narrate --version")
  end
end
EOF
