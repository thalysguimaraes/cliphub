# Security & Privacy

See also: [Architecture](architecture.md), [Known Limitations](limitations.md), [Platform Support](platform-support.md), [Roadmap](roadmap.md), [README](../README.md), [SECURITY.md](../SECURITY.md)

ClipHub is a sensitive product because it moves clipboard contents across devices. The security posture is intentionally simple: trust your tailnet, trust the hub you run inside it, and trust every device you connect to that hub.

## Security model

- The normal deployment boundary is your Tailscale tailnet.
- In tailnet mode, `cliphub` identifies callers through Tailscale/tsnet rather than separate ClipHub accounts or API tokens.
- Every connected client that can reach the hub can receive synced clipboard content.
- The hub is not a blind relay. It reads, stores, and replays clipboard data.

## Privacy posture

- Clipboard contents are stored on the hub in SQLite so reconnecting clients can catch up and history can survive restarts.
- By default, raw clipboard data remains on disk for up to 24 hours and up to 50 history items.
- Content hashes are used for deduplication, but the raw content is also retained because clients need the original payload.
- Clipboard contents are also exposed to the destination device's local clipboard and whatever OS or app integrations that device already permits.
- The iOS companion cache is stored as plain JSON in the shared app-group container.

## Current privacy controls

ClipHub now includes opt-in privacy controls on the desktop agent:

- `--ignore-apps` / `--ignore-processes` keep matching foreground contexts local.
- `--filter-sensitive` can block `secret`, `password-manager`, and `otp` classes.
- `--clear-on-block` can clear the local clipboard when a privacy rule blocks sync.
- `tailclip clear` removes hub clipboard state and persisted history, and `tailclip clear --local` also clears the invoking machine's clipboard.

These controls reduce exposure, but they are not the same as end-to-end secrecy or centrally enforced policy.

## What ClipHub protects well

- It avoids adding another cloud account system or third-party sync service.
- It keeps traffic inside your own tailnet in normal deployments.
- It minimizes identity sprawl by reusing Tailscale's existing device/user trust.

## What ClipHub does not currently protect against

- A compromised or untrusted hub operator. The hub can read synced content.
- A compromised client device. Any synced clipboard is available to that device once applied locally.
- Selective sync or per-device access control. There are no per-item or per-client permissions today.
- End-to-end encryption from source device to destination device.
- At-rest encryption managed by ClipHub itself. Use OS or disk encryption if you need stronger local storage protections.
- Reliable retroactive wipe semantics after content was already synced.
- Perfect context detection. Ignore rules are best-effort and depend on what the local platform can observe.

## Important deployment caveats

### Production use

- Run `cliphub` in normal tailnet mode for real usage.
- Treat every machine running `clipd`, `tailclip`, or the iOS companion as trusted with the same data you would manually copy there.

### Development mode

- `cliphub -dev` is intentionally a local-development mode.
- It defaults to `localhost:8080` and does not use the tailnet identity boundary.
- If you bind it to a wider address, that is your responsibility; it is not the supported secure deployment shape.

### iOS keyboard extension

- The custom keyboard requires "Allow Full Access" to communicate with the hub.
- That requirement materially changes the trust model on the device. Only enable it if you are comfortable with that extension having networked access to clipboard-related data.

## Operator guidance

- Do not sync passwords, one-time codes, private keys, or customer secrets unless every participating device is already trusted for that class of data.
- Prefer full-disk encryption and standard device hardening on any machine that runs the hub.
- Consider lowering `--ttl` and `--max-history` if you want less clipboard retention on disk.
- If you enable privacy filters, treat them as defense-in-depth rather than a guarantee. Verify the specific rules you care about on the platforms you run.
- Use Tailscale ACLs and device hygiene as your primary access-control layer.

## Vulnerability reporting

Private reporting instructions live in [SECURITY.md](../SECURITY.md). Public issues and pull requests are not the right place for security disclosures.
