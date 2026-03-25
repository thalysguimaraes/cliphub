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

### Changed

- `make release` now emits publishable assets under `dist/release`, and `make release-verify` validates checksum and manifest consistency for dry runs and CI.
