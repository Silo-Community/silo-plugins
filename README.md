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
provenance attestation covering every `plugin-<os>-<arch>` binary. The catalog
entry then gains a package-level `bundle_url` beside `checksums_url`. Before
writing the entry, `update-catalog` checks each binary with
`gh attestation verify`. The certificate must name one of two signers:

- Silo's shared release workflow,
  `https://github.com/Silo-Server/.github/.github/workflows/plugin-release.yml@refs/heads/main`.
- A workflow in the plugin's own repository run from the release tag,
  `https://github.com/<repo>/.github/workflows/<file>@refs/tags/<tag>`, where
  `<file>` is a `.yml` or `.yaml` file directly in that directory.

Either way, the tool also requires the issuer
`https://token.actions.githubusercontent.com`, the approved repository as the
source repository, the source ref `refs/tags/<tag>`, a GitHub-hosted runner,
SLSA provenance v1, a Sigstore public-good transparency-log entry, and the same
source commit for every binary. Any failure refuses the release.

Unsigned releases are still accepted for a plugin until its catalog entry has a
`bundle_url`; after that, an unsigned release of that plugin is refused.

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
