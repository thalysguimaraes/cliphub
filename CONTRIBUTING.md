# Contributing to ClipHub

Thanks for investing time in ClipHub. This repository contains the Go hub/agent/CLI code plus the iOS client and extensions, so good contributions are usually focused, well-validated, and explicit about which surfaces they touch.

## Before you start

- Search existing issues and pull requests before opening a new one.
- For larger features, refactors, or behavior changes, open an issue first so the change can be discussed before you spend time implementing it.
- For small fixes and documentation improvements, you can open a pull request directly.
- If you found a security vulnerability, do not open a public issue. Follow [SECURITY.md](SECURITY.md).
- Participation in this project is governed by [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## Development setup

### Core Go services and CLI

ClipHub targets Go 1.21+ and uses the Makefile as the canonical local workflow:

```bash
git clone https://github.com/thalysguimaraes/cliphub.git
cd cliphub
make all
make test
make lint
```

`make all` builds the three shipped binaries:

- `cliphub` for the hub
- `clipd` for the desktop agent
- `tailclip` for CLI access

Clipboard behavior depends on the native tooling described in [README.md](README.md), so if you change clipboard integrations, mention the platform(s) you exercised in your PR.

### iOS work

If you touch anything under [`ios/`](ios/README.md), follow the setup in [ios/README.md](ios/README.md). At minimum, contributors should:

```bash
cd ios
xcodegen generate
```

Then open the generated Xcode project and note the simulator/device validation you ran in the pull request. There is no repo-level automation for the iOS target today, so explicit manual validation notes matter.

## Making a change

1. Create a branch from `main`.
2. Keep the change scoped to a single problem whenever possible.
3. Add or update tests and docs when behavior or workflows change.
4. Update [`CHANGELOG.md`](CHANGELOG.md) when the change is user-facing, operationally meaningful, security-related, or changes the contributor workflow.
5. Run the validation relevant to the files you touched before opening a pull request.

## Pull request expectations

Use the pull request template and make review easy:

- Explain the problem being solved and the chosen approach.
- Link the issue when one exists.
- List the commands and manual checks you ran.
- Include screenshots, terminal output, or reproduction notes when the change affects UX or operational workflows.
- Call out platform coverage when the change touches clipboard behavior, networking, or iOS extensions.

Maintainers may ask for a follow-up issue when a PR expands beyond the original scope.

## Changelog policy

This project keeps a human-maintained changelog in [`CHANGELOG.md`](CHANGELOG.md).

- Add entries to `## Unreleased` in the same pull request that introduces the change.
- Prefer short bullets written from a contributor or operator perspective.
- Use the existing section headings (`Added`, `Changed`, `Fixed`, `Security`) when they fit.

## Questions and reports

- Bugs and feature requests: use the GitHub issue templates.
- Security vulnerabilities: use [SECURITY.md](SECURITY.md).
- Conduct concerns: use [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
