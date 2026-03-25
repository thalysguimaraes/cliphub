# Changelog

All notable changes to this project should be documented in this file.

This changelog is intentionally human-maintained. Update `## Unreleased` in the same pull request whenever a change is user-facing, operationally important, security-sensitive, or changes how contributors work with the repository.

ClipHub does not publish tagged releases yet. Until it does, keep entries under `## Unreleased`. Once tagged releases begin, move those entries into a versioned or dated heading and start a fresh `## Unreleased` section.

Suggested headings:

- `Added`
- `Changed`
- `Fixed`
- `Security`

## Unreleased

### Added

- Governance baseline for contributors and maintainers, including contribution, security, and conduct documentation plus GitHub issue and pull request templates.
- Deterministic release automation that builds publishable archives, writes SHA-256 checksums, generates release notes, and records release metadata for GitHub releases.
- Package-manager release metadata generation for Homebrew, Scoop, and winget, driven by the published release manifest/checksum assets instead of rebuilding binaries.
- Opt-in clipboard privacy controls for app/process ignore lists, sensitive-content filtering (`secret`, `password-manager`, `otp`), and explicit `tailclip clear` history wiping.
- Hub operability endpoints for liveness, readiness, and Prometheus-style metrics, plus reconnect/shutdown integration coverage and a `make test-race` workflow entrypoint.

### Changed

- `make release` now emits publishable assets under `dist/release`, and `make release-verify` validates checksum and manifest consistency for dry runs and CI.
- `make release-package-managers` and `make release-package-managers-verify` now stage and validate the generated Homebrew/Scoop/winget definitions in CI and the tagged release workflow.
- Added raw blob upload/download endpoints, cursor-paged history responses, and typed HTTP error envelopes so large ClipHub API payloads no longer need to rely solely on base64-in-JSON workflows.

### Security

- Documented current privacy limitations, including plaintext-at-rest history storage and the scope of explicit clipboard clear behavior.
