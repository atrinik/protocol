# Metaserver static directory v1

This specification defines the bounded public directory model and canonical
JSON representation served at `https://meta.atrinik.org/index.json`. The model
is independent from gameplay messages, publisher authentication, classic
directory formats, and any optional rendezvous transport. The Protobuf model
in `atrinik.metaserver.v1` is the generated Go/Rust in-memory contract; it is
not a second wire encoding for this endpoint.

## Complete snapshot and limits

A response is one complete immutable snapshot. Partial results, pagination,
patches, and streaming replacement are not part of v1. The body is UTF-8 JSON,
is at most 262,144 bytes including its final LF, and contains at most 512
servers. An empty server list is valid. Receivers MUST validate a replacement
transactionally and retain their previous committed snapshot if validation
fails.

The canonical top-level representation is:

```json
{"schema":"atrinik-directory-v1","generation":"42","generatedAt":"1786219200","expiresAt":"1786233600","servers":[]}
```

The actual bytes have exactly one trailing LF and no other insignificant
whitespace or BOM. Object keys occur in the order shown in this specification.
Strings contain literal UTF-8 except for the JSON-required `\"` and `\\`
escapes. Display strings containing a Unicode control scalar, U+2028, U+2029,
U+FFFE, or U+FFFF are invalid. U+FFFE and U+FFFF are excluded because every
valid snapshot must have an XML 1.0 projection, where those scalars cannot be
represented. No Unicode normalization is performed: the received Unicode
scalar sequence is authoritative and consumers MUST NOT normalize before
comparison or re-encoding. Numbers are base-10 JSON integers without a sign,
fraction, exponent, or leading zero. Values represented as decimal strings
follow the same spelling rule.

`schema` is exactly `atrinik-directory-v1`. Receivers reject the complete body
when it is absent or unknown. `generation` is a canonical decimal string from
1 through 18,446,744,073,709,551,615. A publisher allocates a strictly
increasing generation for every newly published byte sequence and never wraps;
an identical object may retain its generation.

`generatedAt` and `expiresAt` are canonical decimal Unix-second strings from 0
through 253,402,300,799. `expiresAt` is greater than `generatedAt` and at most
14,400 seconds later. At generation time every included server MUST be present,
and `expiresAt` MUST be no later than the earliest backing listing expiry. A
heartbeat may extend backing presence, but an already published snapshot keeps
its original conservative expiry.

A consumer with Unix time `now` treats the snapshot as fresh only when
`generatedAt <= now + 300` and `now < expiresAt`. A future-dated or expired
snapshot is unavailable discovery data, not an authoritative empty directory.
Discovery failure never removes or rewrites a separately configured direct
server endpoint.

## Server representation

Servers are sorted by ascending raw 32-byte `serverId`; duplicates are invalid.
Each server object has these keys in this order:

```text
serverId, certificateSha256, name, description[, region], protocol, content,
players, status, passwordRequired[, endpoint]
```

- `serverId` and `certificateSha256` are identical 64-character lowercase
  hexadecimal SHA-256 digests of the exact DER leaf certificate used by the
  QUIC listener. Their equality is mandatory. The fingerprint is stable
  identity; the optional endpoint is not.
- `name` is 1–80 UTF-8 bytes and `description` is 0–512 UTF-8 bytes. The shared
  display-string restrictions above apply.
- Optional `region` is an opaque lowercase ASCII deployment-region hint of
  1–32 bytes. It starts and ends with an ASCII letter or digit and otherwise
  contains only letters, digits, or hyphens. It is display/filter metadata, not
  geolocation proof.
- `protocol` has exact keys `major`, `minor`. `major` is exactly 1 and `minor`
  is 0–65,535. A client includes the server only if it supports that exact
  Game Protocol version.
- `content` has exact keys `id`, `revisionSha256`. `id` is an opaque lowercase
  ASCII identifier of 1–64 bytes, beginning and ending with a letter or digit
  and otherwise containing only letters, digits, `.`, `_`, or `-`.
  `revisionSha256` is 64-character lowercase hexadecimal. A client includes the
  server only if both values exactly match an installed compatible content
  artifact. Display version text is deliberately not a compatibility input.
- `players` has exact keys `online`, `capacity`. Capacity is 1–100,000; online
  is 0–capacity. The values are aggregate counts and never identify a player.
- `status` is `online`, `full`, or `maintenance`. `online` requires online
  players to be below capacity, `full` requires equality, and `maintenance`
  requires zero online players. Offline and expired servers are omitted.
- `passwordRequired` is a boolean display/connection hint. No password,
  verifier, account fact, or authentication result is directory data.
- Optional `endpoint` has exact keys `hostname`, `port`. The pair is all or
  nothing. Port is 1–65,535. The hostname is a lowercase ASCII DNS name of
  1–253 bytes with at least two 1–63-byte labels, no trailing dot, and only
  letters, digits, and interior hyphens. It MUST contain at least one letter.
  A label beginning with `xn--` MUST be a canonical IDNA A-label:
  non-transitional UTS #46 ToASCII processing with STD3 ASCII, hyphen, joiner,
  bidirectional, and DNS-length checks enabled MUST succeed and return the
  exact same lowercase ASCII label. Unicode U-label input is never accepted.
  IPv4, IPv6, integer, hexadecimal, octal, bracketed, zone-qualified, and
  otherwise numeric IP literals are invalid even if a platform resolver would
  accept them. In particular, a name whose every label is a decimal/octal
  digit string or `0x`-prefixed hexadecimal integer is invalid.

Unknown, missing, duplicated, reordered, differently escaped, or differently
spelled fields make the complete canonical JSON invalid. Implementations may
parse into a typed model and require byte-identical canonical re-encoding.

## Cache identity and conditional retrieval

The strong ETag is the quoted ASCII value
`"atrinik-directory-v1-sha256-{digest}"`, where `digest` is lowercase
hexadecimal SHA-256 of the complete canonical JSON bytes including the final
LF. HTML and XML have their own representation-specific ETags and MUST NOT
reuse the JSON ETag. `Last-Modified` corresponds to `generatedAt`.
The media types are `application/json; charset=utf-8`,
`text/html; charset=utf-8`, and `application/xml; charset=utf-8` for their
respective fixed paths.

Cache freshness MUST NOT extend beyond `expiresAt`. A conditional 304 is valid
only for the same representation and exact generation. Builders publish a new
generation atomically only after every representation is complete; aliases
never expose a partially written generation. Consumers discard a body whose
computed ETag disagrees with a supplied strong ETag.

## HTML and XML semantic projections

`index.html`, `index.xml`, and `index.json` project the same committed server
set, order, generation, timestamps, presence, and values. Their generation
metadata is equal even though representation hashes differ.

HTML is presentation, never a protocol input. Every directory string is
rendered as text with context-appropriate escaping; it cannot inject markup,
URLs, CSS, script, event handlers, or metadata. The page exposes generation and
freshness, marks password-required servers, represents absent region/endpoint
as absent, and does not synthesize an endpoint.

XML uses UTF-8, one `directory` root with `schema`, `generation`,
`generated-at`, and `expires-at` attributes, followed by ordered `server`
elements. Server identity/status/password are attributes; name, description,
optional region, protocol, content, players, and optional endpoint are child
elements in JSON semantic order. XML escaping changes representation bytes but
not field values. Unknown schemas are rejected. This GP1 projection is not the
classic protocol-3 XML route and no consumer may infer classic compatibility
from it.

## Privacy, trust, and compatibility

The schema is exhaustive. Request-source addresses, implicit endpoints,
publisher credentials/signatures/nonces/sequences, rendezvous credentials,
transient candidates, passwords/verifiers, accounts, character state, and
private client or player data are forbidden in every representation, cache
key, log field, and metrics label. The certificate bytes themselves are not
published; only their bound digest is public.

Operational metrics labels are limited to fixed representation/profile names,
bounded outcome codes, and schema versions. Server IDs, names, descriptions,
regions, content IDs/revisions, hostnames, ports, counts, timestamps, ETags,
and arbitrary input never become labels. Diagnostics report only bounded error
classes such as those in the shared fixture and never echo rejected input.

An endpoint is operator-supplied opt-in routing metadata. Publishing it neither
proves control of the DNS name nor changes certificate identity. A server with
no endpoint remains discoverable and may be reached through separately defined
authorized rendezvous or a user-configured address. Direct peer-to-peer QUIC
reveals an endpoint to that authorized peer; relaying is a separate protocol.

Classic discovery remains on its independently versioned schema and route.
Classic fields, OTP flows, numeric protocol IDs, and compatibility inference
never enter this model. Optional rendezvous is omitted from directory v1: its
already independent ticket/capability state machine does not alter static
discovery or configured direct connectivity.

The language-neutral fixture is `fixtures/metaserver-directory-v1.json`. Its
canonical bytes, ETag, semantic projection, boundary recipes, and negative
error classes are normative conformance inputs.
