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
release workflow dispatches `plugin_release_published` to this repository. The
catalog updater verifies the repository and plugin ID against the approval
registry before updating `manifest.json`.

Validate the registry and generated catalog locally with:

```sh
GOWORK=off go test ./...
GOWORK=off go run ./cmd/check-catalog
```

## License

The catalog tooling is licensed under `Apache-2.0`. See [LICENSE](LICENSE).
