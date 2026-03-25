# Security Policy

ClipHub is still pre-release and does not publish tagged releases yet. Security fixes are currently made against the latest commit on `main`.

## Supported versions

| Version | Supported |
| --- | --- |
| `main` | Yes |
| Tagged releases | Not yet applicable |

## Reporting a vulnerability

Please do not report security issues in public GitHub issues, pull requests, or discussions.

Instead, email **eu@thalysguimaraes.com** with a subject like `[ClipHub security] short summary` and include:

- the affected component (`cliphub`, `clipd`, `tailclip`, iOS app/extension, or docs/setup),
- the commit, branch, or binary build you tested,
- clear reproduction steps or a proof of concept,
- the impact you expect if the issue is exploitable,
- any logs, screenshots, or packet captures that help explain the report.

You should receive an acknowledgment within 5 business days. The maintainer will keep the report private while confirming impact, preparing a fix, and coordinating disclosure.

If a report is confirmed, the project may use a private email thread and/or a GitHub security advisory to coordinate the fix and disclosure timeline.

## What counts as a security issue

Examples include:

- bypassing the expected Tailscale-only trust boundary,
- exposing clipboard contents or clipboard history to unauthorized parties,
- code execution, injection, or arbitrary file access triggered by clipboard content,
- privilege escalation in the desktop agents, iOS extensions, or local tooling.

If you are not sure whether something is security-sensitive, report it privately anyway.

## Non-security bugs

If the issue does not need private handling, please use the public GitHub issue templates so the report can be triaged in the open.
