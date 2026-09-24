# Package release setup

The release workflow builds macOS and Linux binaries, Debian packages, and
SHA-256 sidecars from the version in `scripts/cli-version`. Tag that exact
version with a `v` prefix to publish the GitHub release. After publication,
the workflow updates `Formula/narrate.rb` on `main` from the release checksums.

## Debian repository and unattended upgrades

APT publishing is opt-in because the repository needs a long-lived signing
key and GitHub Pages. To enable it:

1. Create a dedicated signing key and keep a backup outside this repository.
2. Add its armored private-key export as the repository Actions secret
   `APT_SIGNING_KEY`. If the key is protected, add its passphrase as
   `APT_SIGNING_PASSPHRASE`.
3. Enable GitHub Pages with **GitHub Actions** as the deployment source.
4. Add the repository Actions variable `PUBLISH_APT_REPO` with value `true`.

For example, `gh secret set APT_SIGNING_KEY < apt-signing-key.asc` uploads the
private signing key to GitHub Actions. Never commit that file. The optional
`publish-apt-repository` job then signs the package indexes and publishes
`https://narrate-it.github.io/narrate/` on each tagged release. The armored
public archive key is published at `narrate-archive-keyring.asc` for apt
clients to install as described in the README.

APT unattended upgrades must also be enabled on each Debian or Ubuntu host.
The README adds Narrate's signed archive origin to the unattended-upgrades
allow list.
