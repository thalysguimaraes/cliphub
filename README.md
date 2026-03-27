<p align="center">
  <img src="https://raw.githubusercontent.com/thalysguimaraes/cliphub/main/assets/logo.png" width="128" height="128" alt="ClipHub">
</p>

<h1 align="center">ClipHub</h1>

<p align="center">
  Clipboard sync across your devices over <a href="https://tailscale.com">Tailscale</a>.<br>
  Copy on one machine, paste on another.
</p>

---

Hub-and-spoke architecture: a lightweight broker runs inside your tailnet, and local agents on each machine watch the clipboard and sync via WebSocket. No cloud, no accounts, no third-party sync service.

## Features

- **Last-write-wins** — central broker keeps sync simple
- **Tailscale for auth** — if you're on the tailnet, you're authorized
- **Rich content** — text, HTML, and images
- **Persistent** — SQLite-backed history survives hub restarts
- **Privacy controls** — ignore apps/processes, filter sensitive content, clear on block
- **Cross-platform** — macOS, Linux, Windows (text-only on Windows for now)
- **iOS companion** — app/keyboard/share extension (manual Xcode setup)

## Components

| Binary | Role |
|--------|------|
| `cliphub` | Hub broker — stores clips in SQLite, fans out updates via WebSocket, runs as a [tsnet](https://tailscale.com/kb/1244/tsnet) node |
| `clipd` | Local agent — watches clipboard, deduplicates by content hash, syncs with hub |
| `tailclip` | CLI — quick get/put/history without running the full agent |

## Install

```bash
go install github.com/thalysguimaraes/cliphub/cmd/cliphub@latest
go install github.com/thalysguimaraes/cliphub/cmd/clipd@latest
go install github.com/thalysguimaraes/cliphub/cmd/tailclip@latest
```

Or build from source:

```bash
git clone https://github.com/thalysguimaraes/cliphub.git
cd cliphub
make all
```

## Usage

```bash
# Start the hub (joins your tailnet as "cliphub")
cliphub

# On each machine, start the agent
clipd

# Or use the CLI directly
tailclip put "hello from the terminal"
tailclip get
tailclip history
tailclip status
```

### Privacy controls

All opt-in:

```bash
clipd -ignore-apps 1Password,Bitwarden -filter-sensitive otp,password-manager
clipd -ignore-processes keepassxc -filter-sensitive secret -clear-on-block
```

## Documentation

- [Architecture](docs/architecture.md) — topology, data flow, persistence
- [Security & Privacy](docs/security.md) — trust boundary, retention model
- [Platform Support](docs/platform-support.md) — canonical support matrix
- [Known Limitations](docs/limitations.md) — current constraints

## License

MIT
