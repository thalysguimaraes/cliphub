<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/thalysguimaraes/cliphub/main/assets/logo.svg">
    <source media="(prefers-color-scheme: light)" srcset="https://raw.githubusercontent.com/thalysguimaraes/cliphub/main/assets/logo.svg">
    <img src="https://raw.githubusercontent.com/thalysguimaraes/cliphub/main/assets/logo.svg" width="128" height="128" alt="ClipHub">
  </picture>
</p>

<h1 align="center">ClipHub</h1>

<p align="center">
  Clipboard sync across your devices over <a href="https://tailscale.com">Tailscale</a>.<br>
  Copy on one machine, paste on another.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/go-%3E%3D1.21-00ADD8?logo=go" alt="Go">
  <img src="https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-333" alt="Platforms">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="License">
</p>

---

ClipHub uses a hub-and-spoke architecture: a lightweight broker runs inside your tailnet, and local agents on each machine watch the clipboard and keep everything in sync via WebSocket. No cloud, no accounts — just your tailnet.

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

## Quick start

```bash
# Install
go install github.com/thalys/cliphub/cmd/cliphub@latest
go install github.com/thalys/cliphub/cmd/clipd@latest
go install github.com/thalys/cliphub/cmd/tailclip@latest

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
```

## Platform support

No CGO required. Clipboard access uses platform-native tools:

| Platform | Text | HTML | Images | Backend |
|----------|:----:|:----:|:------:|---------|
| macOS | yes | yes | yes | `pbcopy`/`pbpaste` + `osascript` (AppKit) |
| Linux (Wayland) | yes | yes | yes | `wl-copy` / `wl-paste` |
| Linux (X11) | yes | yes | yes | `xclip` |
| Windows | yes | - | - | PowerShell clipboard cmdlets |

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
tailclip pause / resume          # toggle sync on this machine
```

Both `clipd` and `tailclip` auto-discover the hub by looking for a `cliphub` peer on your tailnet. Override with `--hub` or `CLIPHUB_HUB`.

## How it works

1. `clipd` polls the local clipboard, reads the richest available type (image > HTML > text), and computes a SHA-256 hash.
2. If the hash or MIME type changed and it's not our own echo, POST it to the hub.
3. The hub assigns a monotonic sequence number, deduplicates by hash + MIME, persists to SQLite, and fans out to all WebSocket subscribers.
4. Other agents receive the update, write it to their clipboard, read back what the platform stored, and mark it as "self-written" to prevent echo loops.
5. On reconnect, agents pass `?since_seq=N` to catch up on anything missed while disconnected.

### Loop prevention

The agent tracks two `(hash, MIME)` pairs:

- **lastWritten** — what we last wrote from a remote update
- **lastSeen** — what we last read from the clipboard

After writing, the agent reads back the clipboard to handle platform format conversion (e.g., HTML written but read back as plain text). Failed sends don't commit state, so the next poll retries automatically.

## API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/clip` | Submit content (`{"content":"...", "mime_type":"text/plain"}` or `{"data":"base64...", "mime_type":"image/png"}`) |
| `GET` | `/api/clip` | Current item (204 if empty) |
| `GET` | `/api/clip/history?limit=N` | History (newest first) |
| `GET` | `/api/clip/stream?since_seq=N` | WebSocket: live updates + catch-up replay |
| `GET` | `/api/status` | Hub status |

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

The agent auto-discovers the hub from your tailnet. No configuration needed.

## Configuration

| Env var | Flag | Default | Description |
|---------|------|---------|-------------|
| `CLIPHUB_HUB` | `--hub` | auto-discovered | Hub URL |
| `CLIPHUB_MAX_HISTORY` | `--max-history` | `50` | Max history items |
| `CLIPHUB_TTL` | `--ttl` | `24h` | Item TTL |

## Roadmap

- [x] Phase 1 — Mac/Linux/Windows, text/plain
- [x] Phase 2 — text/html and images (Mac/Linux)
- [ ] Phase 3 — iOS app with keyboard extension
- [ ] Phase 4 — Security filters, app ignore lists, global pause

## License

[MIT](LICENSE)
