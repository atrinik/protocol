# Game Protocol 1 QUIC transport

This document is normative for Game Protocol 1 transport. The schemas define
control messages; this document defines bytes, state, limits, authorization,
and failure behavior that Protobuf cannot express.

## Security and negotiation

- QUIC uses TLS 1.3 and ALPN `atrinik-game/1`. gRPC is not used.
- The client authenticates the server certificate and pins the SHA-256 digest
  of its SubjectPublicKeyInfo for the selected server identity. A changed pin
  stops before credentials are sent and requires an explicit user decision.
- 0-RTT is disabled for authentication, state-changing actions, and resource
  authorization. Resumption tokens are opaque, single-server, expire within
  24 hours, and never bypass application authentication.
- The client opens one bidirectional control stream and sends `StreamHeader`
  followed by exactly one `ClientHello`. The server answers with exactly one
  `ServerHello` or `ConnectionRejected`. Version/capability rejection occurs
  before credentials or player state are exchanged.

## Framing

Every stream starts with a framed `StreamHeader`. Every subsequent message is
one canonical unsigned LEB128 length followed by exactly that many Protobuf
bytes. The length prefix is at most five bytes, encodes a uint32 without
overflow, has no redundant high zero group, and never spans logical frames.
Zero-length messages, prefixes over five bytes, non-canonical prefixes,
truncation, and trailing partial frames are invalid.

Control, action, and state frames are at most 1,048,576 bytes. Resource frames
are at most 4,194,304 bytes. Compression is not negotiated in version 1, so the
encoded and decompressed limits are identical. A complete message is decoded
and semantically validated into temporary state before commit. Any framing,
decode, bound, authorization, reference, sequence, or revision error discards
that message/transaction and leaves previously committed state unchanged.

## Streams and bounded work

| Role | Opener/direction | Limit | Priority and behavior |
| --- | --- | --- | --- |
| control | client, bidirectional | exactly 1 | highest; negotiation, ping, drain, close |
| actions | client, unidirectional | exactly 1 active | high; 256 messages or 1 MiB queued |
| state | server, unidirectional | exactly 1 per epoch | high; ordered revisions, bounded snapshot transactions |
| resource | client, bidirectional | at most 4 active | low; 8 MiB aggregate queued, cannot consume control/action credit |

Unexpected opener/direction/duplicate roles close the offending stream with
`ERROR_CODE_STREAM_ROLE_INVALID`; a repeated control role closes the
connection. Implementations reserve independent flow-control credit for
control/actions/state so resource traffic cannot starve gameplay. Queue limits
reject or cancel the newest optional work; they never grow memory or block the
simulation owner. QUIC datagrams are unused in version 1.

Idle timeout is negotiated from 10–120 seconds (default 30). Ping is allowed
after half the idle interval. Graceful drain stops admission, communicates a
deadline up to 30 seconds, finishes already committed work, then closes.
Reconnect creates a new session and stream epoch; no action is silently
replayed. Snapshot resumption requires a domain-specific revision contract.

## Close and diagnostic behavior

Application close codes are `0xA7010000 | ErrorCode`. Error classes distinguish
transport, authentication, protocol, content, resource, request validation,
and internal failure. Safe messages are at most 512 UTF-8 bytes and never
contain credentials, tokens, addresses, account existence, unrestricted chat,
private state, packet payloads, filesystem paths, or stack traces. Authentication
failure deliberately does not distinguish unknown accounts from bad secrets.

Malformed input consumes a bounded per-connection strike budget. Oversized
declared frames are rejected before allocation. Servers cap handshakes,
connections, streams, queued bytes, decoding work, and diagnostic rate by
configuration; amplification remains within QUIC validation limits.
