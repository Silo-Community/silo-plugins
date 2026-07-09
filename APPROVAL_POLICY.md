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

Approval may be withdrawn if the plugin becomes unsafe, misleading,
unmaintained, or incompatible. Removing it from the registry and regenerating
the catalog stops new installs and update discovery. Silo does not remotely
disable existing installations.
