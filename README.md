# ClipHub

Clipboard sync across your devices over [Tailscale](https://tailscale.com). Copy on one machine, paste on another.

ClipHub uses a hub-and-spoke architecture: a lightweight broker (`cliphub`) runs inside your tailnet, and local agents (`clipd`) on each machine watch the clipboard and keep everything in sync via WebSocket.

## Why

- **Last-write-wins** — clipboard isn't a collaborative document, so a central broker keeps things simple.
- **Tailscale for auth** — if you're on the tailnet, you're authorized. No passwords, no tokens.
- **Text first** — plain text for now. For files, use [Taildrop](https://tailscale.com/kb/1106/taildrop/).

## Components

| Binary | What it does |
|--------|-------------|
| `cliphub` | Hub broker. Stores current clip + short history, assigns sequence numbers, fans out updates via WebSocket. Runs as a tsnet node (`cliphub.<tailnet>.ts.net`) with automatic HTTPS. |
| `clipd` | Local agent. Polls the clipboard, deduplicates by content hash, sends new items to the hub, applies remote updates without creating feedback loops. |
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

- **macOS**: `pbcopy` / `pbpaste`
- **Linux (Wayland)**: `wl-copy` / `wl-paste`
- **Linux (X11)**: `xclip` or `xsel`
- **Windows**: PowerShell clipboard cmdlets

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
clipd -hub https://cliphub.<your-tailnet>.ts.net
```

Flags:
- `-hub URL` — hub address (also reads `CLIPHUB_HUB` env var)
- `-node NAME` — override node name (defaults to hostname)
- `-poll MS` — poll interval in milliseconds (default: 500)

### CLI

```bash
tailclip get                  # print current clipboard
tailclip put "hello world"    # send text to all devices
echo "piped" | tailclip put   # read from stdin
tailclip history              # show recent clips
tailclip history -n 5         # last 5 clips
tailclip status               # hub uptime, seq, subscribers
tailclip pause                # pause sync on this machine
tailclip resume               # resume sync
```

Use `--hub URL` or set `CLIPHUB_HUB` to point at your hub.

## How it works

1. `clipd` polls the local clipboard every 500ms and computes a SHA-256 hash of the content.
2. If the hash differs from the last seen hash, and it wasn't something we just wrote from a remote update, it's a genuine local change — POST it to the hub.
3. The hub assigns a monotonic sequence number, deduplicates against the current item, stores it in a short history (default 50 items, 24h TTL), and fans out to all connected WebSocket subscribers.
4. Other `clipd` agents receive the update, write it to their local clipboard, and mark the hash as "self-written" so the next poll tick doesn't echo it back.

### Loop prevention

The agent tracks two hashes:
- **lastWrittenHash**: what we last wrote to the clipboard (from a remote update)
- **lastSeenHash**: what we last read from the clipboard

This dual-hash approach prevents the feedback loop where writing a remote update triggers a "new clipboard content" event that gets sent back to the hub.

## Hub API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/clip` | Submit content. Body: `{"content":"..."}` |
| `GET` | `/api/clip` | Get current item (204 if empty) |
| `GET` | `/api/clip/history?limit=N` | Get history |
| `GET` | `/api/clip/stream` | WebSocket stream of updates |
| `GET` | `/api/status` | Hub status |

## Configuration

| Env var | Flag | Default | Description |
|---------|------|---------|-------------|
| `CLIPHUB_HUB` | `--hub` | `http://localhost:8080` | Hub URL (for clipd/tailclip) |
| `CLIPHUB_MAX_HISTORY` | `--max-history` | `50` | Max history items |
| `CLIPHUB_TTL` | `--ttl` | `24h` | Item TTL |

## Roadmap

- [x] Phase 1: Mac/Linux/Windows, text/plain
- [ ] Phase 2: text/html and small images
- [ ] Phase 3: iOS app with keyboard extension
- [ ] Phase 4: Security filters, app ignore lists, global pause

## License

MIT
