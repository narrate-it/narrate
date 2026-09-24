#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: publish-apt-repo.sh DIST_DIR SITE_DIR" >&2
  exit 2
fi
: "${APT_SIGNING_KEY:?set APT_SIGNING_KEY to an armored private key}"
dist=$1
site=$2
command -v dpkg-scanpackages >/dev/null 2>&1 || { echo "dpkg-scanpackages is required" >&2; exit 1; }
command -v apt-ftparchive >/dev/null 2>&1 || { echo "apt-ftparchive is required" >&2; exit 1; }

gnupg_home=$(mktemp -d)
chmod 700 "$gnupg_home"
trap 'rm -rf "$gnupg_home"' EXIT HUP INT TERM
export GNUPGHOME=$gnupg_home
printf '%s\n' "$APT_SIGNING_KEY" | gpg --batch --import
key_id=$(gpg --batch --with-colons --list-secret-keys | awk -F: '$1 == "sec" {print $5; exit}')
[ -n "$key_id" ] || { echo "APT_SIGNING_KEY contains no private signing key" >&2; exit 1; }

mkdir -p "$site/pool/main/n/narrate" \
  "$site/dists/stable/main/binary-amd64" "$site/dists/stable/main/binary-arm64"
for arch in amd64 arm64; do
  package=$dist/narrate_linux_${arch}.deb
  [ -f "$package" ] || { echo "missing Debian package: $package" >&2; exit 1; }
  cp "$package" "$site/pool/main/n/narrate/"
done

(cd "$site" && dpkg-scanpackages --arch amd64 pool /dev/null > dists/stable/main/binary-amd64/Packages)
(cd "$site" && dpkg-scanpackages --arch arm64 pool /dev/null > dists/stable/main/binary-arm64/Packages)
gzip -n -9 -kf "$site/dists/stable/main/binary-amd64/Packages"
gzip -n -9 -kf "$site/dists/stable/main/binary-arm64/Packages"
(cd "$site" && apt-ftparchive \
  -o APT::FTPArchive::Release::Origin=Narrate \
  -o APT::FTPArchive::Release::Label=Narrate \
  -o APT::FTPArchive::Release::Suite=stable \
  -o APT::FTPArchive::Release::Codename=stable \
  -o APT::FTPArchive::Release::Architectures="amd64 arm64" \
  -o APT::FTPArchive::Release::Components=main \
  release dists/stable > dists/stable/Release)
gpg --batch --yes --pinentry-mode loopback --passphrase "${APT_SIGNING_PASSPHRASE:-}" \
  --local-user "$key_id" --clearsign --output "$site/dists/stable/InRelease" "$site/dists/stable/Release"
gpg --batch --yes --pinentry-mode loopback --passphrase "${APT_SIGNING_PASSPHRASE:-}" \
  --local-user "$key_id" --detach-sign --output "$site/dists/stable/Release.gpg" "$site/dists/stable/Release"
gpg --batch --armor --export "$key_id" > "$site/narrate-archive-keyring.asc"
