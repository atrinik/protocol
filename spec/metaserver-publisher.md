# Metaserver publisher authentication v1

This specification defines Atrinik's certificate-bound, one-request
metaserver publisher authentication profile. It is independent from gameplay
messages and from static directory representations. RFC 9421 HTTP Message
Signatures and RFC 9530 `Content-Digest` are normative dependencies.

## Identity and profiles

The publisher identity is the lowercase hexadecimal SHA-256 digest of the
exact DER-encoded leaf certificate used by the server's QUIC listener. Every
publish body carries that DER certificate using canonical padded base64. The
path, `Atrinik-Server-ID` field, body server ID, certificate fingerprint, and
HTTP signature `keyid` parameter MUST be identical. The certificate's public
key MUST be ECDSA P-256. Certificate chain trust and wall-clock validity are
not identity inputs: clients pin the exact DER digest, and proof of the
matching private key authorizes publication.

Two profiles are deliberately separate:

| Profile | Exact path | Signature `tag` |
| --- | --- | --- |
| Classic | `/v1/classic/servers/{server-id}/publish` | `atrinik-classic-publish-v1` |
| Game Protocol 1 | `/v1/servers/{server-id}/publish` | `atrinik-game-publish-v1` |

Both use signature label `atrinik`, algorithm parameter
`ecdsa-p256-sha256`, and `Content-Type: application/json`. The normalized
lowercase `@authority` distinguishes production, canary, and local
environments. A signature for one authority, path, or tag is invalid in every
other environment or protocol profile.

## Request and signature

The request is `POST` only, has no query or fragment, has no
`Content-Encoding`, and carries between 1 and 4096 body bytes. The body bytes
are digested and signed exactly as transmitted; parsed JSON is never
reserialized for authentication. The only accepted `Content-Digest` member is
lowercase `sha-256` with one canonical padded-base64 Byte Sequence.

### Classic v1 body

The classic body is UTF-8 JSON with no BOM, insignificant whitespace,
trailing bytes, duplicate keys, or alternate key order. Its keys occur exactly
in this order:

```text
schema, serverId, certificate, name, playersCount, version, textComment,
public, passwordRequired[, hostname, port]
```

`schema` is `atrinik-classic-publish-v1`; `serverId` is the identity described
above; and `certificate` is canonical padded base64 of at most 2,048 DER bytes.
`name` is 1–80 UTF-8 bytes, `version` is 1–32 UTF-8 bytes, and `textComment` is
0–256 UTF-8 bytes. Those strings are not Unicode-normalized and exclude U+0000
through U+001F and U+007F. They contain only valid Unicode scalar values;
unpaired UTF-16 surrogate escapes are rejected. Non-ASCII characters remain
their literal UTF-8 JSON representation, while only quotation mark and reverse
solidus require escaping. `playersCount` is a canonical JSON integer from 0 through
4294967295. `public` and `passwordRequired` are JSON booleans.

`hostname` and `port` are either both absent or both present. The hostname is
a lowercase ASCII DNS name of 1–253 bytes, has at least two labels, uses only
letters, digits, and interior hyphens, and has neither a trailing dot nor a
port. Each label is 1–63 bytes. A label beginning with `xn--` satisfies the
canonical IDNA A-label rule in the
[static directory specification](metaserver-directory.md), including
byte-identical non-transitional UTS #46 ToASCII validation. The port is a
canonical JSON integer from 1 through 65535. A publisher that has not explicitly
opted into a DNS endpoint omits both fields; request-source and discovered
numeric addresses are never substituted.

### Game Protocol 1 body

The Game Protocol 1 body is UTF-8 JSON subject to the same no-BOM,
no-whitespace, no-trailing-byte, no-duplicate-key, and exact-key-order rules as
the classic body. Its keys occur exactly in this order:

```text
schema, serverId, certificate, name, description[, region], protocol, content,
players, status, public, passwordRequired[, endpoint]
```

`schema` is `atrinik-game-publish-v1`; `serverId` and `certificate` use the
same identity and certificate encoding as the classic body. `name`,
`description`, optional `region`, `protocol`, `content`, `players`, `status`,
`passwordRequired`, and optional `endpoint` have exactly the scalar,
relationship, canonical hostname, and Unicode rules of one `DirectoryServer`
in the [static directory specification](metaserver-directory.md). `public` is
a JSON boolean. The publisher body deliberately omits directory generation and
freshness values: the directory publisher assigns those independently after a
successful atomic state transition.

`region` is omitted when absent. `endpoint` is omitted when no explicit DNS
endpoint is configured; it is an object whose keys occur exactly in the order
`hostname, port`. Request-source and discovered numeric addresses are never
substituted. Nested object key order is exactly `major, minor` for `protocol`,
`id, revisionSha256` for `content`, and `online, capacity` for `players`.

A private body (`public: false`) is authenticated and advances replay and
presence state, but the receiver deletes any public directory entry and stores
none of its display, region, player, status, content, or direct-endpoint fields
as public directory state. A later public body must republish the complete
public model.

The language-neutral Game Protocol 1 body, signature, private-publication, and
negative conformance vectors are in
`fixtures/metaserver-game-publisher-v1.json`. The normative JSON Schema is
`schema/metaserver-game-publisher-v1.schema.json`; canonical ordering, UTF-8
byte limits, certificate identity, status/player relationships, and IDNA
semantics remain requirements even where JSON Schema cannot express them.

The covered components occur in this exact order:

```text
("@method" "@authority" "@path" "content-digest" "content-type" "atrinik-server-id" "atrinik-publish-sequence")
```

The `Signature-Input` value has this exact parameter order:

```text
atrinik=(...);created={unix};expires={unix+300};nonce="{32-lower-hex}";alg="ecdsa-p256-sha256";keyid="{server-id}";tag="{profile-tag}"
```

`created` and `expires` are non-negative RFC 8941 Integers and `expires` is
exactly 300 seconds after `created`. The verifier accepts the request only
while its clock is no more than 300 seconds before `created` and not after
`expires`. The nonce is a freshly generated, nonzero 128-bit value. The
publish sequence is a canonical decimal unsigned 64-bit integer in the range
1 through 18446744073709551615, with no leading zero, carried identically in
the signed field and authenticated body state where a schema includes it.

The signature base is ASCII, joins lines with one LF, and has no trailing LF:

```text
"@method": POST
"@authority": {normalized authority}
"@path": {exact path}
"content-digest": {exact Content-Digest field value}
"content-type": application/json
"atrinik-server-id": {server-id}
"atrinik-publish-sequence": {sequence}
"@signature-params": {Signature-Input value after `atrinik=`}
```

ECDSA output uses the RFC 9421 `ecdsa-p256-sha256` encoding: 32-byte
big-endian, zero-padded `r` followed by 32-byte `s`. `Signature` is exactly
`atrinik=:{canonical-padded-base64}:`.

Receivers MUST reject a missing, duplicated, combined, differently cased,
unexpected, or non-canonical signature field or parameter. They MUST also
reject extra signature dictionary members, unsupported algorithms/tags,
invalid certificate encodings, non-P-256 keys, and any disagreement among the
signed identity values. An intermediary-combined duplicate scalar field does
not match the field's strict grammar and is rejected.

## Replay and atomic publication

Before attempting network I/O, the publisher atomically persists the sequence
value reserved for that request as its local high-water mark. An ambiguous
result consumes that sequence; a retry reserves both a higher sequence and a
fresh nonce. Gaps are valid. Sequence state is non-secret but must be stored
atomically beside the persistent server identity and included in backup/restore
policy.

Only after certificate and signature verification may a receiver charge an
identity quota or inspect the body as authoritative input. For each server
identity, one serialized atomic publication operation:

1. rejects a nonce already present in the bounded replay window;
2. requires `sequence` to be greater than the stored sequence;
3. records the sequence and nonce;
4. applies owner, listing, presence, and visible-revision state; and
5. rotates and stores the hash of any returned rendezvous credential.

A failed or replayed operation changes none of those values. A replay conflict
returns HTTP 409 with stable code `publish_replay` and a decimal string
`minimumNextSequence`. The client atomically advances local state to at least
`minimumNextSequence - 1` before reserving a new request, so the new sequence is
at least the advertised minimum. A minimum of zero is invalid. If either the
receiver's stored sequence or the publisher's local high-water mark is the
uint64 maximum, publication for that identity is exhausted and requires
explicit operator recovery; it never wraps and no larger minimum can be
represented.

Accepted nonces are retained for at least 24 hours, bounded by the authenticated
48-publishes-per-UTC-day identity limit and pruned after expiry. Sequence
ordering remains authoritative after nonce pruning. An authenticated heartbeat
with equivalent public content refreshes presence but does not advance the
visible directory revision. A public-content change or expiry does.

Successful responses and all 409/429 responses use `Cache-Control: no-store`.
The success body for both profiles is exactly
`{"status":"ok","rendezvousToken":"{64-lower-hex}"}`. A replay conflict is
exactly
`{"error":{"code":"publish_replay","minimumNextSequence":"{uint64}"}}`.
Rate-limit errors use the deployment's bounded error envelope, return HTTP 429,
and carry matching delta-seconds in both `Retry-After` and
`error.retry_after_seconds`; publishers treat that response as a consumed
attempt and do not reuse its nonce or sequence. The rendezvous credential
appears only in a successful authenticated response, is hashed at rest, and is
never logged. Credentials, signatures,
nonces, certificate bodies, signed fields, sequence envelopes, request-source
addresses, and response credentials are prohibited from logs, metrics labels,
and directory artifacts.

## Compatibility and ownership

This is a new, fail-closed profile. Classic OTP/COTP requests are a temporary
compatibility route and are not accepted on either v1 path. Their removal
requires a deployed publisher hostname, supported-consumer cutover, and a
documented rollback window. Game Protocol 1 never uses the classic form, key,
route, schema, or signature tag.

The protocol repository owns these canonical inputs and the shared fixture.
The metaserver Worker owns strict HTTP parsing, authentication, replay storage,
atomic listing mutation, rate limits, and redaction. Classic and Go servers own
private-key access, crash-safe sequence persistence, request construction,
backoff, and response handling.
