# Silo Approved Community Plugin Catalog

This repository publishes the catalog index for community-maintained Silo
plugins that have been approved for inclusion.

Approval means Silo maintainers have validated that a released plugin installs,
works as described, and is considered safe for its documented use at the time
of review. The plugin remains maintained and supported by its community
maintainers.

## Catalog URL

`https://raw.githubusercontent.com/Silo-Community/silo-plugins/main/manifest.json`

Silo administrators enable this catalog with **Include approved community
plugins**. They do not need to add the URL manually.

## Approval and releases

Only repositories listed in [`approved-plugins.json`](approved-plugins.json)
can publish entries. See [`APPROVAL_POLICY.md`](APPROVAL_POLICY.md) for the
review requirements.

After an approved plugin publishes a checksum-bearing GitHub release, its
release workflow dispatches `plugin_release_published` to this repository.

## Catalog updates

`update-catalog` refuses a release unless all of these hold, and writes
`manifest.json` only after the last check:

- The repository is `owner/name` and the tag is `v<major>.<minor>.<patch>`,
  optionally with a `-suffix`. Both are checked before either goes into a URL.
- The repository is listed in `approved-plugins.json`, spelled exactly as it is
  there, and the release's `manifest.json` names the plugin ID approved for it.
  A dispatch cannot add a repository or change which plugin it publishes.
- The version is newer than the plugin's listed version. Re-running for the
  listed version does nothing and succeeds. A deliberate rollback needs the
  `allow_rollback` input of a manual `workflow_dispatch` run; a
  `repository_dispatch` cannot set it.
- The release is published for exactly that tag and is neither a draft nor a
  prerelease. There is no override for drafts or prereleases.
- Every catalogued asset URL (the `plugin-<os>-<arch>` binaries,
  `checksums.txt`, and `plugin.sigstore.json`) is
  `https://github.com/<repo>/releases/download/<tag>/<asset name>`. Other
  release assets, such as `.zip` archives, are ignored.

`manifest.json` is read at a commit SHA, never at the tag name: the commit the
signature names for a signed release, or the commit the tag points at
(following an annotated tag) for an unsigned one.

Entries whose plugin is no longer in `approved-plugins.json` are removed the
next time the catalog is written.

### Signed releases

A signed release carries `plugin.sigstore.json`: a Sigstore bundle with one SLSA
provenance attestation covering every `plugin-<os>-<arch>` binary. Before
writing the entry, `update-catalog` downloads every binary and checks it with
`gh attestation verify`. The entry then gains a package-level `bundle_url`
beside `checksums_url`, and each binary gains a `checksum`: the SHA-256 of the
file the tool verified. Silo servers prefer that checksum over `checksums_url`,
so the listed bytes stay pinned even if the release assets are replaced later.
Unsigned entries carry no per-binary checksum.

The certificate must name exactly one signer, chosen by the plugin's entry in
`approved-plugins.json`:

- By default, Silo's shared release workflow,
  `https://github.com/Silo-Server/.github/.github/workflows/plugin-release.yml@refs/heads/main`.
  No other workflow is accepted.
- If the entry sets `signer_workflow` (for example `"signer_workflow":
  "release.yml"`), that workflow file in the plugin's own repository, run from
  the release tag:
  `https://github.com/<repo>/.github/workflows/<signer_workflow>@refs/tags/<tag>`.
  The shared workflow is then not accepted for that plugin. The signed commit
  must also be on the repository's default branch: GitHub's compare API must
  report it as identical to or behind that branch, not ahead or diverged.

For both signers, the tool also requires the issuer
`https://token.actions.githubusercontent.com`, the approved repository as the
source repository, the source ref `refs/tags/<tag>`, a GitHub-hosted runner,
SLSA provenance v1, a Sigstore public-good transparency-log entry, and the same
source commit for every binary. Any failure refuses the release.

What a signature proves:

- Shared workflow: the binaries were built on a GitHub-hosted runner by Silo's
  release workflow at `main`, from the approved repository at the commit the
  release tag named. The build steps are Silo's, not the plugin's. It does not
  prove the plugin's source at that commit was reviewed.
- Own workflow: the binaries were built on a GitHub-hosted runner by the named
  workflow file, as it existed at the tagged commit, from a commit already on
  the default branch. Anyone who can push to the default branch and create
  tags can change that workflow and what it builds, so this proves origin from
  the approved repository's default branch, not that the build steps or source
  were reviewed.

Unsigned releases are still accepted for a plugin until its catalog entry has a
`bundle_url`; after that, an unsigned release of that plugin is refused.
`signer_workflow` is an approval decision; see
[`APPROVAL_POLICY.md`](APPROVAL_POLICY.md). `check-catalog` validates its format.

### Workflow

The update workflow has no concurrency group. Runs race on the push, and each
attempt rebuilds the entry on the current `main` and runs `check-catalog`, so a
duplicate run is a no-op and an older release that loses the race to a newer
one is refused. `CATALOG_PUSH_TOKEN` pushes the update; the workflow's own
token has no permissions, because every approved repository is public.

### Running it locally

Pass the repository in `owner/name` form and the exact release tag. Verifying a
signed release needs the `gh` CLI on `PATH`. To try the tool against a release
the catalog already lists, rebuild that entry in a copy of the catalog:

```sh
catalog_copy="$(mktemp)"
cp manifest.json "$catalog_copy"
GOWORK=off go run ./cmd/update-catalog \
  -repo Silo-Community/silo-plugins-requests-arr \
  -tag v0.1.11 \
  -allow-rollback \
  -manifest "$catalog_copy"
diff manifest.json "$catalog_copy"
```

Without `-allow-rollback`, the listed version does nothing and an older one is
refused.

## Development

Validate the registry and generated catalog locally with:

```sh
GOWORK=off go test ./...
GOWORK=off go run ./cmd/check-catalog
```

See [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.

## License

The catalog tooling is licensed under `Apache-2.0`. See [LICENSE](LICENSE).
