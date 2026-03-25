# Platform Support

See also: [Architecture](architecture.md), [Security & Privacy](security.md), [Known Limitations](limitations.md), [Roadmap](roadmap.md), [README](../README.md), [iOS setup](../ios/README.md)

## Canonical support statement

The main supported ClipHub sync path today is the desktop stack:

- `cliphub` as the hub,
- `clipd` as the clipboard-watching agent,
- `tailclip` as the direct CLI,
- across macOS, Linux, and Windows.

An iOS companion app, keyboard extension, and share extension also exist in this repository, but they are currently source-available/manual-build components rather than a fully packaged, parity-with-desktop support tier.

## Desktop clipboard capability matrix

| Platform | `clipd` status | `text/plain` | `text/html` | `image/png` | Notes |
| --- | --- | :---: | :---: | :---: | --- |
| macOS | supported | yes | yes | yes | Uses `pbcopy`/`pbpaste` plus AppKit-backed AppleScript for rich content. |
| Linux (Wayland) | supported | yes | yes | yes | Requires `wl-copy` and `wl-paste`. |
| Linux (X11) | supported | yes | yes | yes | Requires `xclip`. |
| Windows | supported with reduced fidelity | yes | no | no | Current backend uses PowerShell clipboard cmdlets and only handles plain text. |

## Component support by surface

| Surface | Current state | Notes |
| --- | --- | --- |
| `cliphub` | usable from source on macOS, Linux, and Windows; current release artifact target is `linux/amd64` | CI smoke-builds the hub on Ubuntu, macOS, and Windows, but the `make release` target only emits a Linux AMD64 hub binary today. |
| `clipd` | first-class desktop agent on macOS/Linux; reduced-fidelity support on Windows | Rich HTML/image clipboard parity is not there on Windows yet. |
| `tailclip` | supported on macOS, Linux, and Windows | It talks directly to the hub and is less constrained by local clipboard APIs than `clipd`. |
| iOS app + keyboard + share extension | source available, manual setup | Requires iOS 17+, Tailscale connectivity, Xcode setup, and user-managed signing. No packaged release or CI pipeline is defined here. |

## iOS scope today

The iOS codebase currently covers:

- a container app for current clip, history, and settings,
- a custom keyboard that can paste the current hub clip,
- a share extension that can send content to the hub.

That is useful, but it is not the same product shape as the desktop background agent. When this repository says "cross-platform" today, it means:

- desktop sync across macOS, Linux, and Windows is the primary supported path,
- iOS is an in-repo companion path with manual setup and a narrower interaction model.

## Choosing a deployment target

- If you want the smoothest end-user setup today, use the desktop stack.
- If you want an iPhone companion for your own tailnet and are comfortable with Xcode/manual signing, the iOS codebase is usable as a source-first project.
- If you need rich clipboard fidelity on Windows, wait for future work before treating that platform as equivalent to macOS/Linux.
