# Approved Community Plugin Policy

An approved community plugin must satisfy all of the following at review time:

- Its source repository is public and belongs to `Silo-Community`.
- The plugin builds and its test suite passes without local dependency replaces.
- A reviewer has exercised the documented connection, setup, and primary
  behavior against a supported Silo version.
- The manifest and README accurately describe behavior, configuration,
  external services, and network access.
- The source and release workflow contain no known unsafe or deceptive behavior.
- Releases provide supported-platform binaries and SHA-256 checksums through
  the standard Silo plugin release workflow.
- The repository declares a license, a support path, and active maintainers or
  CODEOWNERS.

Approval records the evidence URL and date in `approved-plugins.json`. A release
dispatch cannot approve a repository by itself.

## Release signing

Signed releases of an approved plugin must be signed by Silo's shared release
workflow (`Silo-Server/.github`, `plugin-release.yml` at `main`) unless its
registry entry sets `signer_workflow`. That optional field names one workflow
file in the plugin's own repository, as a plain file name ending in `.yml` or
`.yaml` (for example `"signer_workflow": "release.yml"`). When it is set, the
catalog accepts only that workflow, run from the release tag, for a commit on
the repository's default branch, and no longer accepts the shared workflow for
that plugin.

Setting `signer_workflow` is an approval decision. The shared workflow fixes the
build steps; a plugin's own workflow lets its maintainers change them. Before
setting it, a reviewer checks that the named workflow builds and attests the
release binaries, runs only on GitHub-hosted runners, and is triggered by
release tags, and that the repository protects its default branch. Approval
records this review the same way as the original approval.

Approval may be withdrawn if the plugin becomes unsafe, misleading,
unmaintained, or incompatible. Removing it from the registry and regenerating
the catalog stops new installs and update discovery. Silo does not remotely
disable existing installations.
