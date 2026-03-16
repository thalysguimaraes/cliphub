# ClipHub

Clipboard sync across your devices over [Tailscale](https://tailscale.com). Copy on one machine, paste on another.

ClipHub uses a hub-and-spoke architecture: a lightweight broker (`cliphub`) runs inside your tailnet, and local agents (`clipd`) on each machine watch the clipboard and keep everything in sync via WebSocket.

## Why

- **Last-write-wins** — clipboard isn't a collaborative document, so a central broker keeps things simple.
- **Tailscale for auth** — if you're on the tailnet, you're authorized. No passwords, no tokens.
- **Rich content** — text, HTML, and images. For files, use [Taildrop](https://tailscale.com/kb/1106/taildrop/).

## Components

| Binary | What it does |
|--------|-------------|
| `cliphub` | Hub broker. Stores current clip + short history, assigns sequence numbers, fans out updates via WebSocket. Runs as a tsnet node (`cliphub.<tailnet>.ts.net`) with automatic HTTPS. |
| `clipd` | Local agent. Polls the clipboard, deduplicates by content hash and MIME type, sends new items to the hub, applies remote updates without creating feedback loops. |
| `tailclip` | CLI. Quick access to get/put/history without running the full agent. |

## Install

```bash
go install github.com/thalys/cliphub/cmd/cliphub@latest
go install github.com/thalys/cliphub/cmd/clipd@latest
go install github.com/thalys/cliphub/cmd/tailclip@latest
```

Or build from source:

```bash
git clone https://github.com/thalysguimaraes/cliphub.git
cd cliphub
make all        # builds bin/cliphub, bin/clipd, bin/tailclip
```

### Platform support

Clipboard access uses platform-native tools — no CGO required:

| Platform | Text | HTML | Images | Tools used |
|----------|------|------|--------|------------|
| **macOS** | yes | yes | yes | `pbcopy`/`pbpaste`, `osascript` (AppKit) |
| **Linux (Wayland)** | yes | yes | yes | `wl-copy`/`wl-paste` |
| **Linux (X11)** | yes | yes | yes | `xclip` |
| **Windows** | yes | — | — | PowerShell `Get-Clipboard`/`Set-Clipboard` |

## Usage

### Start the hub

On any machine in your tailnet:

```bash
# Production: creates a tsnet node with automatic HTTPS
cliphub

# Development: plain HTTP on localhost
cliphub -dev
cliphub -dev -addr localhost:9090
```

The hub becomes reachable at `https://cliphub.<your-tailnet>.ts.net`.

### Start the agent

On each machine you want to sync:

```bash
# Auto-discovers the hub by querying tailscale for a "cliphub" peer
clipd

# Or specify explicitly
clipd -hub https://cliphub.<your-tailnet>.ts.net
```

Flags:
- `-hub URL` — hub address (auto-discovered from tailnet, or `CLIPHUB_HUB` env var)
- `-node NAME` — this node's name (auto-discovered from tailscale, or hostname)
- `-poll MS` — poll interval in milliseconds (default: 500)

### CLI

```bash
tailclip get                       # print current clipboard
tailclip get -o image.png          # save binary clip to file
tailclip put "hello world"         # send text to all devices
echo "piped" | tailclip put        # read from stdin
tailclip put --file photo.png      # send a file (MIME auto-detected)
tailclip put --mime text/html "<b>bold</b>"  # explicit MIME type
tailclip history                   # show recent clips
tailclip history -n 5              # last 5 clips
tailclip status                    # hub uptime, seq, subscribers
tailclip pause                     # pause sync on this machine
tailclip resume                    # resume sync
```

Both `clipd` and `tailclip` auto-discover the hub by looking for a `cliphub` node on your tailnet. Override with `--hub URL` or `CLIPHUB_HUB` env var.

## How it works

1. `clipd` polls the local clipboard every 500ms, reads the richest available type (image > HTML > plain text), and computes a SHA-256 hash of the content.
2. If the hash or MIME type differs from the last seen state, and it wasn't something we just wrote from a remote update, it's a genuine local change — POST it to the hub.
3. The hub assigns a monotonic sequence number, deduplicates against the current item (by hash + MIME type), stores it in a short history (default 50 items, 24h TTL), and fans out to all connected WebSocket subscribers.
4. Other `clipd` agents receive the update, write it to their local clipboard, read back what the platform actually stored, and mark the result as "self-written" so the next poll tick doesn't echo it back.

### Loop prevention

The agent tracks two pairs of (hash, MIME type):
- **lastWritten**: what we last wrote to the clipboard (from a remote update)
- **lastSeen**: what we last read from the clipboard

After writing a remote update, the agent reads back the clipboard to capture any format conversion the platform may have done (e.g., writing HTML but the platform exposes it as plain text on the next read). This ensures the next poll won't see a false change.

## Hub API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/clip` | Submit content. Body: `{"content":"...", "mime_type":"text/plain"}` or `{"data":"base64...", "mime_type":"image/png"}` |
| `GET` | `/api/clip` | Get current item (204 if empty) |
| `GET` | `/api/clip/history?limit=N` | Get history |
| `GET` | `/api/clip/stream` | WebSocket stream of updates |
| `GET` | `/api/status` | Hub status |

Text types use the `content` field (string). Binary types use `data` (base64-encoded). If `mime_type` is omitted, defaults to `text/plain`.

## Running as a service

### Linux (systemd)

```bash
# Hub
sudo cp bin/cliphub /usr/local/bin/
sudo cp init/systemd/cliphub.service /etc/systemd/system/
sudo systemctl enable --now cliphub

# Agent (user service — needs graphical session for clipboard access)
cp bin/clipd /usr/local/bin/
cp init/systemd/clipd.service ~/.config/systemd/user/
systemctl --user enable --now clipd
```

### macOS (launchd)

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

The agent auto-discovers the hub from your tailnet. No configuration needed.

## Configuration

| Env var | Flag | Default | Description |
|---------|------|---------|-------------|
| `CLIPHUB_HUB` | `--hub` | auto-discovered | Hub URL (for clipd/tailclip) |
| `CLIPHUB_MAX_HISTORY` | `--max-history` | `50` | Max history items |
| `CLIPHUB_TTL` | `--ttl` | `24h` | Item TTL |

## Roadmap

- [x] Phase 1: Mac/Linux/Windows, text/plain
- [x] Phase 2: text/html and images (Mac/Linux)
- [ ] Phase 3: iOS app with keyboard extension
- [ ] Phase 4: Security filters, app ignore lists, global pause

## License

MIT
