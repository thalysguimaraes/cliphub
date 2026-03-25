# Public Go SDK Scope

This note records the recommendation for `CLA-283` and scopes what should
become public when ClipHub promotes a Go client out of `internal/hubclient`.
It is intentionally a design boundary, not the SDK implementation itself.

## Recommendation

Ship the first public Go SDK from this repository, in the existing module,
instead of creating a separate Go module or repo.

Recommended import path:

```go
github.com/thalysguimaraes/cliphub/hubclient
```

Why this should stay in-repo first:

- The current transport contract is still owned and exercised here: the hub
  handlers, `tailclip`, `clipd`, and `internal/hubclient` all evolve together.
- Paged history, blob upload/download, and typed error envelopes were just
  introduced as coordinated transport changes. Splitting the SDK now would add
  cross-repo release choreography before the public contract has much history.
- The repository already tags releases for binaries and API-affecting changes,
  so the first public SDK can inherit the same release train and changelog.
- Promoting `hubclient` in-place keeps migration small for first-party callers,
  because the package name can stay the same while the import path becomes
  public.

Create a separate module later only if one of these becomes true:

- the SDK needs an independent release cadence from the hub/CLI binaries,
- external users need compatibility windows that differ from the main repo,
- or the public client grows enough package surface that repo-level tagging is
  no longer a good fit.

## Proposed Public Surface

The initial public SDK should cover the stable request/response HTTP flows and
their typed errors. It should be intentionally smaller than
`internal/hubclient`.

Public entry points:

- `Config`
- `Client`
- `New(Config) (*Client, error)`
- `(*Client).BaseURL() string`
- `(*Client).Current(ctx context.Context) (*Item, error)`
- `(*Client).Put(ctx context.Context, req PutRequest) (*Item, error)`
- `(*Client).HistoryPage(ctx context.Context, limit int, cursor string) (*HistoryPage, error)`
- `(*Client).Download(ctx context.Context, seq uint64) (*Blob, error)`
- `(*Client).Clear(ctx context.Context) error`
- `ErrNoCurrentClip`
- `*Error` (typed HTTP error)

Public transport types should be stable, non-internal equivalents of the
current HTTP payloads:

- `Item` for `GET /api/clip` and `PUT` responses
- `Summary` for paged-history/blob metadata
- `HistoryPage` for `/api/clip/history/page`
- `Blob` for raw `/api/clip/blob` downloads
- `Error` for structured error envelopes plus the HTTP `statusCode`

`PutRequest` should describe only the stable upload contract: MIME type plus
either text content or raw bytes. The current dev-only source override header
should not be part of the first public request shape.

The public SDK should align with the transport contracts that now matter most:

- `POST /api/clip/blob` is the preferred upload path for binary payloads and
  large HTML bodies.
- `GET /api/clip/blob?seq=N` is the stable raw-download path.
- `GET /api/clip/history/page` is the primary history traversal API.
- Structured error envelopes are part of the contract and should be surfaced as
  typed Go errors instead of stringly status handling.

## Explicitly Out Of Scope For The First Public SDK

These pieces should stay private until they have a stronger compatibility story:

- WebSocket message framing and reconnect helpers (`protocol.WSMessage`,
  `since_seq` orchestration, and the current agent-side stream loop)
- `Status()` and its current `map[string]any` payload, because the status schema
  is not typed or scoped as a public contract yet
- dev-only source header overrides (`PutRequest.Source` / `X-Clip-Source`)
- URL/path construction helpers and other low-level transport plumbing
- clipboard, discovery, privacy, and agent orchestration helpers

The compatibility `GET /api/clip/history` array endpoint may remain in
first-party/internal use, but it should not define the first public history API.
If it is exposed at all, it should be clearly documented as a compatibility
helper rather than the preferred contract.

## Package-Shape Constraints

The public package should not expose `internal/protocol` types directly.

Before anything ships publicly:

- promote or copy the stable HTTP DTOs out of `internal/protocol`,
- keep WebSocket-only message types private,
- keep helper methods that are implementation details (`do`, endpoint joining,
  header wiring) unexported,
- and ensure any exported types only describe documented HTTP contracts.

This keeps the public SDK aligned with the API surface that external callers can
actually rely on, instead of freezing first-party implementation details.

## Versioning And Compatibility Policy

Versioning should follow the main repository tags.

Until the module reaches `v1`, the SDK is explicitly pre-v1:

- public API changes are allowed when the underlying transport contract changes,
- but every breaking change must be called out in release notes,
- and the preferred transport paths above should not churn without a documented
  reason.

The bar for `v1` should be:

- paged history cursor semantics are stable,
- blob upload/download endpoints are stable,
- typed error codes/messages are intentionally curated,
- and the exported Go types have survived at least one release cycle without
  needing structural changes.

After `v1`, normal Go module semver rules apply:

- additive response fields are compatible,
- removing/renaming exported fields or methods is a breaking change,
- changing error-code semantics is a breaking change,
- changing cursor semantics or the preferred blob/history contracts is a
  breaking change.

Compatibility target:

- one SDK major version maps to one hub API major version,
- first-party binaries in the repo may use internal helpers while the public SDK
  remains narrower,
- and expanding the public surface later is acceptable as long as new entry
  points are additive.
