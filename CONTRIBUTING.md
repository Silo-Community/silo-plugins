# Contributing to the Approved Community Plugin Catalog

This repository holds the catalog of approved community plugins and the tooling
that builds it. Plugin implementation changes belong in the plugin's own
repository. Approval requests and the review rules are covered by
[`APPROVAL_POLICY.md`](APPROVAL_POLICY.md).

## Before you start

Open an [issue](https://github.com/Silo-Community/silo-plugins/issues) before
changing catalog schema, the approval registry format, release ingestion,
signature policy, or automation.

Catalog entries are generated from a plugin's published GitHub release by the
update workflow. Do not hand-edit `manifest.json` to work around a missing or
incorrect upstream artifact. Changes to `approved-plugins.json` follow the
approval policy and are reviewed by its code owner.

## Development setup

Use the Go version declared in `go.mod`. The update command reads release assets
from GitHub, so use a real tagged release when testing ingestion, and run it
against a copy of `manifest.json` as the README shows. Never place tokens in
command arguments, committed files, or logs.

## Validate your change

```sh
GOWORK=off go test ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
GOWORK=off go run ./cmd/check-catalog
gofmt -l .
```

`gofmt -l .` should print nothing. For catalog output changes, inspect the
complete `manifest.json` diff and verify repository URLs, versions, platforms,
the checksum URL, the bundle URL when the release is signed, capabilities,
presentation metadata, and approval metadata against the source release and
the registry.

Changes to `update-catalog`'s refusal rules (input validation, the approval
registry lookup, version order, asset URLs, the commit binding, or signature
policy) need a unit test for each refusal in `catalog/catalog_test.go`.

## Open the pull request

Use a Conventional Commit title, explain the catalog or automation impact, and
paste the actual validation results. Disclose AI assistance as the
[AI-assisted contribution policy](https://github.com/Silo-Server/silo-server/blob/main/docs/ai-contributions.md)
describes.
