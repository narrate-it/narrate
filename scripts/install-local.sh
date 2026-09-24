#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
data_root=${XDG_DATA_HOME:-${HOME:?HOME is required}/.local/share}
install_dir=$data_root/narrate
bin_dir=${HOME:?HOME is required}/.local/bin
mkdir -p "$install_dir" "$bin_dir"
tmp=$install_dir/.narrate.new
trap 'rm -f "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
(cd "$repo" && go build -trimpath -o "$tmp" .)
chmod 755 "$tmp"
mv -f "$tmp" "$install_dir/narrate"
install -m 755 "$repo/scripts/narrate.sh" "$install_dir/narrate.sh"
install -m 644 "$repo/scripts/cli-version" "$install_dir/cli-version"
ln -sfn "$install_dir/narrate.sh" "$bin_dir/narrate"
printf 'Installed narrate to %s\n' "$bin_dir/narrate"
printf 'The launcher checks for newer releases every six hours.\n'
