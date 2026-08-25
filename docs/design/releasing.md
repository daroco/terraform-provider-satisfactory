# Releasing to the Terraform Registry

The `release` workflow (`.github/workflows/release.yml`) runs GoReleaser on any
pushed `v*` tag and produces the artifacts the
[Terraform Registry](https://registry.terraform.io/) ingests: per-platform
zips, a `SHA256SUMS` file, its GPG signature, and the
`terraform-registry-manifest.json` (protocol 6). These are one-time setup steps
the repository owner must do before the first release; they need credentials and
registry access that CI does not have.

## One-time setup

1. **Generate a GPG signing key** (RSA 4096, no expiry is simplest):

   ```sh
   gpg --full-generate-key
   gpg --armor --export-secret-keys <KEY_ID> > private.asc
   gpg --armor --export <KEY_ID>              # public key, for the registry
   ```

2. **Add GitHub Actions secrets** on the repo (Settings → Secrets and variables
   → Actions):
   - `GPG_PRIVATE_KEY` — the contents of `private.asc`.
   - `PASSPHRASE` — the key's passphrase (omit the input if the key has none).

   `GITHUB_TOKEN` is provided automatically; no PAT is required.

3. **Publish the provider on the registry**: sign in at
   registry.terraform.io with the `daroco` GitHub account, add the provider,
   and upload the **public** GPG key under the account's signing keys. The
   source address is `daroco/satisfactory`.

## Cutting a release

1. Move the `Unreleased` entries in [`CHANGELOG.md`](../../CHANGELOG.md) under a
   new version heading and commit.
2. Regenerate and commit docs if any schema/example changed: `make docs`.
3. Tag and push:

   ```sh
   git tag v0.1.0
   git push origin v0.1.0
   ```

The workflow builds, signs, and creates the GitHub Release; the registry picks
it up within a few minutes.

## Local dry run

Verify the build without publishing (needs `goreleaser` installed and
`GPG_FINGERPRINT` set, or use `--skip=sign`):

```sh
goreleaser release --snapshot --clean --skip=sign
```
