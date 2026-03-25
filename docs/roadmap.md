# Roadmap

See also: [Architecture](architecture.md), [Security & Privacy](security.md), [Known Limitations](limitations.md), [Platform Support](platform-support.md), [README](../README.md)

This roadmap focuses on the gaps that matter most for adoption. It is intentionally aligned with the current codebase rather than an aspirational rewrite.

## Current baseline

- Desktop hub, agent, and CLI exist and are exercised in CI.
- macOS and Linux currently have the richest clipboard fidelity.
- Windows desktop support exists, but only for plain text clipboard sync today.
- An iOS companion app, keyboard extension, and share extension already exist in the repository, but they are still a source-first/manual-build path.
- Opt-in privacy controls already exist for ignore lists, sensitive-content filters, and explicit clear behavior.

## Near-term priorities

- Turn the iOS companion into a clearly supported path:
  - define packaging/signing expectations,
  - narrow the parity story relative to desktop,
  - document the security and user-experience trade-offs clearly.
- Harden privacy controls beyond the current opt-in baseline:
  - make coverage and platform caveats clearer,
  - improve detection quality where foreground context is weak,
  - tighten cleanup semantics and operator guidance.

## Medium-term gaps

- Improve Windows clipboard fidelity beyond plain text.
- Expand release engineering so supported binaries and support claims line up more closely.
- Tighten operator controls around retention, disclosure, and sensitive-data handling.

## What this roadmap is not promising yet

- End-to-end encryption that hides clipboard contents from the hub.
- A full peer-to-peer architecture without a trusted broker.
- File-sync workflows that replace Taildrop.

If those capabilities are essential for your use case today, ClipHub is not there yet. See [Known Limitations](limitations.md) and [Security & Privacy](security.md) before adopting it for sensitive workflows.
