<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/thalysguimaraes/cliphub/main/assets/logo.svg?v=2">
    <source media="(prefers-color-scheme: light)" srcset="https://raw.githubusercontent.com/thalysguimaraes/cliphub/main/assets/logo.svg?v=2">
    <img src="https://raw.githubusercontent.com/thalysguimaraes/cliphub/main/assets/logo.svg?v=2" width="128" height="128" alt="ClipHub">
  </picture>
</p>

<h1 align="center">ClipHub</h1>

<p align="center">
  Clipboard sync across your devices over <a href="https://tailscale.com">Tailscale</a>.<br>
  Copy on one machine, paste on another.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/go-%3E%3D1.21-00ADD8?logo=go" alt="Go">
  <a href="https://github.com/thalysguimaraes/cliphub/actions/workflows/ci.yml"><img src="https://github.com/thalysguimaraes/cliphub/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
  <img src="https://img.shields.io/badge/desktop-macOS%20%7C%20Linux%20%7C%20Windows-333" alt="Desktop platforms">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="License">
</p>

---

ClipHub uses a hub-and-spoke architecture: a lightweight broker runs inside your tailnet, and local agents on each machine watch the clipboard and keep everything in sync via WebSocket. No cloud, no accounts, and no third-party sync service.

The main supported path today is desktop sync across macOS, Linux, and Windows. This repository also contains an iOS companion app/keyboard/share-extension project with manual setup; see the docs below for the current support boundary.

## Why

- **Last-write-wins** — clipboard isn't a collaborative document, so a central broker keeps things simple.
- **Tailscale for auth** — if you're on the tailnet, you're authorized. No passwords, no tokens.
- **Rich content** — text, HTML, and images. For files, use [Taildrop](https://tailscale.com/kb/1106/taildrop/).
- **Persistent** — SQLite-backed history survives hub restarts. Agents catch up on reconnect via `since_seq`.

## Components

| Binary | What it does |
|--------|-------------|
| `cliphub` | Hub broker. Stores current clip + short history (SQLite), assigns sequence numbers, fans out updates via WebSocket. Runs as a [tsnet](https://tailscale.com/kb/1244/tsnet) node with automatic HTTPS. |
| `clipd` | Local agent. Polls the clipboard every 500ms, deduplicates by content hash + MIME type, sends new items to the hub, applies remote updates without creating feedback loops. |
| `tailclip` | CLI. Quick access to get/put/history without running the full agent. |

## Documentation

- [Architecture](docs/architecture.md) explains the hub/agent/CLI/iOS topology, data flow, persistence, and transport modes.
- [Security & Privacy](docs/security.md) defines the trust boundary, retention model, and what ClipHub does not protect against yet.
- [Known Limitations](docs/limitations.md) lists the current behavior, content-type, packaging, and operational constraints.
- [Platform Support](docs/platform-support.md) is the canonical support statement for desktop platforms and the iOS companion.
- [Roadmap](docs/roadmap.md) tracks the adoption-critical gaps still planned.

## Quick start

```bash
# Install
go install github.com/thalysguimaraes/cliphub/cmd/cliphub@latest
go install github.com/thalysguimaraes/cliphub/cmd/clipd@latest
go install github.com/thalysguimaraes/cliphub/cmd/tailclip@latest

# Start the hub (joins your tailnet as "cliphub")
cliphub

# On each machine, start the agent (auto-discovers the hub)
clipd

# Or use the CLI
tailclip put "hello from the terminal"
tailclip get
```

### Build from source

```bash
git clone https://github.com/thalysguimaraes/cliphub.git
cd cliphub
make all    # builds bin/cliphub, bin/clipd, bin/tailclip
make test   # runs all tests
make test-race  # runs the race detector across the Go packages
make release VERSION=v0.1.1-rc1  # writes deterministic release assets to dist/release
```

## CI

Public CI runs on pull requests and pushes to `main`.

- `go test ./...` runs on Ubuntu, macOS, and Windows.
- `go build ./cmd/clipd ./cmd/cliphub ./cmd/tailclip` runs on Ubuntu, macOS, and Windows as a native multi-binary smoke build.
- `go vet ./...` runs on Ubuntu as the stable built-in lint gate.
- `make test-race` (`go test -race ./...`) runs on Ubuntu as the race-detection gate.
- `make release VERSION=ci-snapshot` plus `make release-verify VERSION=ci-snapshot` run on Ubuntu to smoke-test the deterministic release artifacts.
- `make release-package-managers VERSION=ci-snapshot` plus `make release-package-managers-verify VERSION=ci-snapshot` run on Ubuntu to dry-run the Homebrew, Scoop, and winget metadata generated from the release manifest/checksums.

Platform-specific exclusions:

- Race detection is only required on `ubuntu-latest`; that keeps a single supported race gate in CI while still covering the full package set.
- Release archives are generated from a single Linux host. The current target matrix ships `cliphub` for `linux/amd64` and ships `clipd`/`tailclip` for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, and `windows/amd64`.

## Releasing

Tagged releases are published by `.github/workflows/release.yml`. The workflow builds deterministic archives, writes release notes and SHA-256 checksums, verifies the generated manifest, uploads the core release assets, then generates Homebrew/Scoop/winget definitions from the published manifest/checksum assets and uploads those definitions to the same GitHub release.

See [`docs/releases.md`](docs/releases.md) for the full dry-run and publish flow.

## Platform support

Desktop sync is the main supported path today:

- macOS and Linux support `text/plain`, `text/html`, and `image/png`.
- Windows support is currently limited to `text/plain` in the desktop clipboard backend.
- The repository also contains an iOS companion app/keyboard/share extension with manual Xcode setup rather than a packaged release flow.

See [Platform Support](docs/platform-support.md) for the canonical matrix and the exact iOS support boundary.

## Usage

### Hub

```bash
cliphub                          # production: tsnet with auto HTTPS
cliphub -dev                     # development: localhost:8080, plain HTTP
cliphub -dev -addr :9090         # custom dev address
```

The hub joins your tailnet as `cliphub` and becomes reachable at `https://cliphub.<your-tailnet>.ts.net`. State is persisted to `~/.config/cliphub/tsnet/clips.db`.

### Agent

```bash
clipd                            # auto-discovers hub from tailnet
clipd -hub http://100.x.x.x     # explicit hub address
clipd -poll 200                  # faster polling (ms)
clipd -ignore-apps 1Password,Bitwarden -filter-sensitive otp,password-manager
clipd -ignore-processes keepassxc -filter-sensitive secret -clear-on-block
```

### CLI

```bash
tailclip get                     # print current clipboard
tailclip get -o image.png        # save binary clip to file
tailclip put "hello"             # send text
echo "piped" | tailclip put      # from stdin
tailclip put --file photo.png    # send file (MIME auto-detected)
tailclip put --mime text/html "<b>bold</b>"
tailclip history                 # recent clips
tailclip history -n 5
tailclip status                  # hub uptime, seq, subscribers
tailclip clear                   # clear shared clipboard state + hub history
tailclip clear --local           # also clear this machine's local clipboard
tailclip pause / resume          # toggle sync on this machine
```

Both `clipd` and `tailclip` auto-discover the hub by looking for the hostname in `CLIPHUB_HOSTNAME` on your tailnet (`cliphub` by default). Override the full URL with `--hub` or `CLIPHUB_HUB`.

### Privacy controls

Privacy controls are **opt-in**. By default, ClipHub keeps syncing every clipboard item it can read.

- `--ignore-apps` / `CLIPHUB_IGNORE_APPS`: comma-separated foreground app names or bundle IDs that should never sync.
- `--ignore-processes` / `CLIPHUB_IGNORE_PROCESSES`: comma-separated process names that should never sync.
- `--filter-sensitive` / `CLIPHUB_FILTER_SENSITIVE`: comma-separated sensitive classes to block from sync. Supported classes are `secret`, `password-manager`, and `otp`.
- `--clear-on-block` / `CLIPHUB_CLEAR_ON_BLOCK`: when a privacy rule blocks sync, also clear the local clipboard on that machine.

Notes:

- App/process detection is best-effort. On Linux, ignore lists and the `password-manager` class currently depend on `xdotool` being available so `clipd` can inspect the active window.
- `secret` and `otp` filters are content-based and continue to work even when app/process detection is unavailable.

## How it works

At a high level:

1. `clipd` watches the local clipboard and sends new content to the hub.
2. `cliphub` stores the current item plus short history, assigns a sequence number, and fans updates out over WebSocket.
3. Other clients apply the change locally and recover missed items via `since_seq` after reconnects.

See [Architecture](docs/architecture.md) for the full data flow, trust boundaries, persistence model, and loop-prevention details.

## API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/clip` | Submit JSON content. Text stays inline as `content`; binary remains supported as base64-in-JSON for compatibility. |
| `POST` | `/api/clip/blob` | Submit raw request bytes with `Content-Type` set to the clip MIME type. Preferred for large HTML or binary payloads. |
| `GET` | `/api/clip` | Current item (204 if empty) |
| `GET` | `/api/clip/blob?seq=N` | Download the current clip or a specific history entry as raw bytes. |
| `GET` | `/api/clip/history?limit=N` | Compatibility history endpoint (newest first, bare array response). |
| `GET` | `/api/clip/history/page?limit=N&cursor=SEQ` | Cursor-paged history with lightweight items and `next_cursor` metadata. |
| `DELETE` | `/api/clip` | Clear current hub clipboard state and persisted history |
| `GET` | `/api/clip/stream?since_seq=N` | WebSocket: live updates + catch-up replay |
| `GET` | `/api/status` | Hub status, readiness, and lightweight counters |
| `GET` | `/healthz` | Liveness check (200 while the process is healthy) |
| `GET` | `/readyz` | Readiness check (503 while shutting down/draining) |
| `GET` | `/metrics` | Prometheus-style text metrics for hub activity and lifecycle |

### Large payload flows

The original JSON API still works, but it expands binary payloads because the
body has to be base64-encoded before it can be embedded in JSON.

Example: a 1 MiB PNG becomes `1,398,104` base64 characters before JSON framing.

Legacy JSON upload:

```bash
curl -X POST http://localhost:8080/api/clip \
  -H 'Content-Type: application/json' \
  -d '{"data":"<base64>","mime_type":"image/png"}'
```

Preferred raw blob flow:

```bash
# Upload raw bytes with the real MIME type.
curl -X POST http://localhost:8080/api/clip/blob \
  -H 'Content-Type: image/png' \
  --data-binary @photo.png

# Download the raw bytes back later.
curl http://localhost:8080/api/clip/blob?seq=123 -o photo.png
```

The blob endpoint returns lightweight metadata (sequence, hash, size, and a
download path) so large uploads do not bounce back through another base64 JSON
response.

### History pagination

Use `/api/clip/history/page` when clients need to walk a large history window
without pulling the entire response as one array or embedding binary payloads in
every item.

```bash
curl 'http://localhost:8080/api/clip/history/page?limit=2'
```

```json
{
  "items": [
    {
      "seq": 42,
      "mime_type": "image/png",
      "hash": "...",
      "source": "laptop",
      "created_at": "2026-03-25T17:00:00Z",
      "expires_at": "2026-03-26T17:00:00Z",
      "size_bytes": 2048,
      "download_path": "/api/clip/blob?seq=42"
    }
  ],
  "next_cursor": "41",
  "has_more": true
}
```

Pass the returned `next_cursor` back as `cursor` to fetch the next page. The
paged endpoint uses SQLite-backed history when persistence is enabled, so it can
navigate beyond the in-memory `maxHistory` ring buffer.

### Typed errors

HTTP failures now return a structured JSON envelope that clients can branch on:

```json
{
  "error": {
    "code": "invalid_cursor",
    "message": "cursor must be a positive integer",
    "details": {
      "cursor": "abc"
    }
  }
}
```

### Go SDK note

First-party Go callers continue to use `internal/hubclient` while the HTTP
contract settles. A public Go SDK/package decision is intentionally tracked as a
separate follow-up rather than being bundled into this transport update.

## Running as a service

<details>
<summary>Linux (systemd)</summary>

```bash
# Hub
sudo cp bin/cliphub /usr/local/bin/
sudo cp init/systemd/cliphub.service /etc/systemd/system/
sudo systemctl enable --now cliphub

# Agent (user service)
cp bin/clipd /usr/local/bin/
cp init/systemd/clipd.service ~/.config/systemd/user/
systemctl --user enable --now clipd
```
</details>

<details>
<summary>macOS (launchd)</summary>

```bash
# Hub
sudo cp bin/cliphub /usr/local/bin/
cp init/launchd/com.cliphub.hub.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.cliphub.hub.plist

# Agent
sudo cp bin/clipd /usr/local/bin/
cp init/launchd/com.cliphub.agent.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.cliphub.agent.plist
```
</details>

The agent auto-discovers the hub from your tailnet by default. Set `CLIPHUB_HOSTNAME` only if you run the hub under a different tailnet hostname.

## Configuration

| Env var | Flag | Default | Description |
|---------|------|---------|-------------|
| `CLIPHUB_HUB` | `--hub` | auto-discovered | Hub URL |
| `CLIPHUB_IGNORE_APPS` | `--ignore-apps` | empty | Comma-separated app names or bundle IDs to keep local |
| `CLIPHUB_IGNORE_PROCESSES` | `--ignore-processes` | empty | Comma-separated process names to keep local |
| `CLIPHUB_FILTER_SENSITIVE` | `--filter-sensitive` | empty | Comma-separated sensitive classes to block (`secret,password-manager,otp`) |
| `CLIPHUB_CLEAR_ON_BLOCK` | `--clear-on-block` | `false` | Clear the local clipboard when a privacy rule blocks sync |
| `CLIPHUB_HOSTNAME` | `--hostname` (hub only) | `cliphub` | Tailnet hostname used by the hub and auto-discovery |
| `CLIPHUB_MAX_HISTORY` | `--max-history` | `50` | Max history items |
| `CLIPHUB_TTL` | `--ttl` | `24h` | Item TTL |

## Security and privacy limitations

- Privacy filters are disabled by default. You must opt in with `clipd` flags or environment variables.
- Hub history is stored as plain SQLite at `~/.config/cliphub/tsnet/clips.db`. The iOS cache is stored as plain JSON in the shared app-group container. ClipHub currently relies on OS-level disk encryption and file permissions rather than application-layer history encryption.
- `tailclip clear` clears hub state and persisted history explicitly. Add `--local` if you also want to clear the current machine's system clipboard.
- Clearing hub state does not retroactively wipe clipboard contents that were already written to other devices or stale offline caches. Clear those local clipboards separately if you need a full cleanup.
- App/process ignore rules are best-effort because they depend on foreground-window inspection. On Linux, that currently requires `xdotool`.

## Roadmap

The next adoption-critical gaps are:

- turning the existing iOS companion into a clearly supported path,
- hardening the new privacy controls with clearer coverage, defaults, and cleanup semantics,
- improving Windows clipboard fidelity and release packaging parity.

See [Roadmap](docs/roadmap.md) for the current framing and [Known Limitations](docs/limitations.md) for the gaps that still apply today.

## Contributing

Contributions are welcome. Start with [CONTRIBUTING.md](CONTRIBUTING.md) for setup, validation, and pull request expectations.

- Use the GitHub issue templates for bugs and feature requests.
- Update [CHANGELOG.md](CHANGELOG.md) when a change is notable to users, operators, or contributors.
- Follow [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) in all project spaces.

## Security

See [Security & Privacy](docs/security.md) for the product trust model and [SECURITY.md](SECURITY.md) for private vulnerability reporting instructions.

## License

[MIT](LICENSE)
