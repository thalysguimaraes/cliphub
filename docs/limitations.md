# Known Limitations

See also: [Architecture](architecture.md), [Security & Privacy](security.md), [Platform Support](platform-support.md), [Roadmap](roadmap.md), [README](../README.md)

ClipHub is intentionally small and opinionated. The current behavior favors predictable sync over platform-perfect fidelity.

## Behavior and consistency

- Sync is last-write-wins. Two devices copying at nearly the same time will not merge content.
- `clipd` is poll-based, not event-driven. The default poll interval is 500 ms.
- Reconnect recovery is sequence-based. Clients catch up from `since_seq`, but there is no multi-version history or conflict resolution model.
- Remote writes are read back after apply to handle platform conversions, which means the stored representation can differ from what the source client originally wrote.

## Content-type limitations

- The protocol caps individual clipboard payloads at 10 MiB.
- Files are not synced as files. Use [Taildrop](https://tailscale.com/kb/1106/taildrop/) for that workflow.
- Rich content support is platform-dependent:
  - macOS and Linux can exchange `text/plain`, `text/html`, and `image/png`.
  - Windows currently supports `text/plain` only in the desktop clipboard backend.
- Clipboard format conversion can degrade content. For example, a platform may round-trip HTML as plain text.

## Platform and packaging limitations

- Linux requires either `wl-copy`/`wl-paste` or `xclip`.
- The iOS companion is source-available and manually built from Xcode; there is no packaged release flow or CI coverage for it in this repository today.
- The iOS experience is not equivalent to `clipd` on desktop. The app/keyboard/share extension can read from or send to the hub, but there is no always-on iOS background clipboard watcher.
- The hub currently cross-builds release artifacts only for `linux/amd64`, even though source builds are smoke-tested more broadly in CI.

## Security and policy limitations

- The hub is trusted with raw clipboard contents.
- There are no per-app ignore lists, no secret filtering, no selective sync rules, and no policy engine yet.
- Development mode is not a hardened network deployment path.

## Operational limitations

- Auto-discovery depends on Tailscale metadata. If discovery fails, clients fall back to localhost-oriented behavior unless you set an explicit hub URL.
- History retention is short by design. ClipHub is a sync tool, not a long-term clipboard archive.

The planned work to address the biggest gaps is tracked in [Roadmap](roadmap.md).
