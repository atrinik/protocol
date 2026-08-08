# Game Protocol 1 common values

This specification is normative together with
`proto/atrinik/game/v1/common.proto`. Generated parsers do not enforce these
semantic limits; producers validate before encoding and consumers validate a
complete message before changing live state.

## Identity and digest rules

| Type | Encoding and bound | Scope and lifetime |
| --- | --- | --- |
| `AccountId`, `CharacterId` | exactly 16 opaque bytes, never all zero | persistent across reconnect and catalog reload |
| `SessionId` | exactly 16 opaque, unpredictable bytes | one authenticated connection lifecycle |
| `MapInstanceId` | exactly 16 opaque bytes | one loaded map-instance lifecycle |
| `RequestId`, `TransactionId` | exactly 16 unpredictable bytes | one session; duplicates are idempotently rejected or replay the recorded result |
| `ActionId`, `EntityId` | nonzero 64-bit slot plus nonzero 32-bit generation | one server process; generation changes before slot reuse |
| `ContentId`, `ResourceId` | namespace 1–32 lowercase ASCII characters; value 1–160 characters in slash-separated lowercase ASCII segments | stable within the named catalog/resource contract |
| `Digest256`, `DiagnosticId` | exactly 32 and 16 bytes respectively | digest algorithm is SHA-256; diagnostics are opaque and safe to disclose |

Database primary keys, filesystem paths, localized names, pointers, renderer
handles, GPU slots, and enum positions never cross the wire as identities.
Namespaces and value segments start and end with `[a-z0-9]`; interior
characters may additionally be `._-`. Namespaces contain no slash. Values use
single slashes between nonempty segments, so leading/trailing slash, empty
segments, `.`/`..`, and hidden/path-confusable segments are invalid.

## Revisions, clocks, and numeric values

- Revisions and simulation ticks are unsigned, monotonic within their owning
  object or process epoch, start at one, and fail on overflow. Stale or skipped
  revisions require rejection or a fresh snapshot; wraparound is forbidden.
- `WallTimestamp` uses Unix UTC seconds and 0–999,999,999 nanoseconds. It is
  used only for calendar-visible facts, never simulation ordering or deadlines.
- `DurationMillis` is monotonic elapsed time, is bounded to 30 days unless a
  domain contract declares a smaller maximum, and saturating conversion is
  forbidden.
- Money is signed integral minor currency units. Quantity is unsigned. Domain
  contracts define narrower maxima; arithmetic rejects overflow and never uses
  floating point. Rounding is round-half-to-even only where a formula contract
  explicitly requires division.

## Coordinates and bounded data

Tile coordinates and offsets are signed 32-bit values with a semantic range of
−1,048,576 through 1,048,575. Physical levels are −32 through 31. Surface IDs
are 0 through 255 and meaningful only inside a map instance. Viewports are at
least 1×1 and at most 512×512 tiles.

Unless a domain says less, strings are valid UTF-8, contain no NUL, and are at
most 4,096 encoded bytes; identifiers are ASCII as specified above. Byte fields
are at most 1 MiB, lists/maps at most 4,096 entries, page sizes at most 256, and
cursors at most 256 opaque bytes. Nested message depth is at most 32. Unknown
fields are retained by relays only when explicitly required and otherwise
ignored. Unknown enum values are never treated as a known default; required
unknown values reject the containing transaction.

Proto3 scalar absence and the encoded default are equivalent. Message fields
use presence. A required semantic message field that is absent is invalid even
when Protobuf decoding succeeds. Removed fields and enum values are reserved
and never reused.
