# Architecture

See also: [Security & Privacy](security.md), [Known Limitations](limitations.md), [Platform Support](platform-support.md), [Roadmap](roadmap.md), [README](../README.md)

ClipHub is built around a single clipboard broker inside your tailnet plus multiple clients that either watch the clipboard (`clipd`) or talk to the hub directly (`tailclip`, the iOS companion).

## System layout

```text
local clipboard <-> clipd ----------------------+
                                                |
tailclip ------------------------------------+  |
                                             |  v
iOS app / share extension / keyboard --> cliphub --> SQLite history
                                             ^
                                             |
                               WebSocket stream + REST API
```

## Core components

| Component | Responsibility |
| --- | --- |
| `cliphub` | Central broker. Assigns sequence numbers, stores the current clip plus history in SQLite, exposes REST + WebSocket APIs, and uses Tailscale/tsnet identity in normal mode. |
| `clipd` | Desktop agent. Polls the local clipboard, chooses the richest available MIME type, sends new content to the hub, and applies remote updates without echo loops. |
| `tailclip` | Direct CLI client for `get`, `put`, `history`, `status`, `pause`, and `resume`. It talks to the hub without running a background clipboard watcher. |
| `ios/ClipHub`, `TailPasteKeyboard`, `TailClipShare` | Source-available iOS companion. The app shows current/history/settings, the keyboard pastes the current hub clip, and the share extension sends content to the hub. |

## Data flow

1. A client discovers the hub URL from Tailscale metadata unless `--hub` or `CLIPHUB_HUB` overrides it.
2. `clipd` polls the local clipboard every 500 ms and reads the richest available content in this order: `image/png`, `text/html`, then `text/plain`.
3. `clipd` can optionally apply local privacy rules before upload. Blocked items stay local and can optionally clear the local clipboard.
4. The client hashes the content and MIME type. If the item is new and not one the client just wrote itself, it sends the item to `cliphub`.
5. `cliphub` stores the clip, assigns a monotonic `seq`, and broadcasts it to WebSocket subscribers.
6. Other clients receive the update, apply it locally, read back what the OS actually stored, and mark that result as self-written so the next poll does not loop the same item back to the hub.
7. Reconnecting clients can resume from `since_seq` to catch up on missed items.

## Persistence and retention

- The hub persists clipboard history in SQLite.
- In the default tailnet mode, the database lives at `~/.config/cliphub/tsnet/clips.db`.
- Retention defaults to a 24 hour TTL and a history depth of 50 items, both configurable on the hub.
- The hub stores both hashes and raw clipboard content because it needs to replay clipboard data, not just detect duplicates.

## Transport modes

### Normal mode

- `cliphub` runs as a `tsnet` node inside your tailnet.
- When Tailscale HTTPS is enabled, the hub serves HTTPS on its MagicDNS hostname.
- If HTTPS is not enabled, the hub falls back to plain HTTP on the tailnet.

### Development mode

- `cliphub -dev` listens on localhost by default and skips the tailnet identity layer.
- It is useful for local development only, not for network-exposed deployments.

## Privacy controls in the architecture

- Privacy controls are currently enforced in `clipd`, before content is uploaded to the hub.
- Operators can opt in to app-ignore, process-ignore, and sensitive-content filters.
- Those controls are local and best-effort. They do not retroactively delete content that was already synced to other devices or cached elsewhere.

## Why the architecture is centralized

ClipHub intentionally uses a hub-and-spoke design instead of peer-to-peer merge logic:

- clipboard state is naturally last-write-wins,
- clients can reconnect and catch up from a single sequence source,
- the hub can keep short history and status information in one place,
- clients stay simple and only need REST/WebSocket connectivity.

That simplicity comes with an explicit trade-off: the hub is trusted with clipboard contents and metadata. See [Security & Privacy](security.md) for the trust model and [Known Limitations](limitations.md) for behavior and portability caveats.
