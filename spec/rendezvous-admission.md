# Rendezvous admission v1

This normative MIT contract makes computational admission mandatory for Classic
client rendezvous. Tracking anchor: metaserver-worker PR173,
`discussion_r4241990064`. It supplements [access tokens](access-tokens.md).
It does not change Protobuf or authenticated server-control frames. Consumers
are metaserver-worker Core and room, Classic library and client. Game rendezvous
remains disabled until its own coordinated consumer exists. Deployment and merge
require separate authorization.

## Challenge and canonical values

Use the exact configured rendezvous authority, never a directory-supplied origin.
Map either the ordinary `wss` Classic client URL or protected access rendezvous URL
to `https` on that same authority and POST
`/v1/rendezvous/challenge/classic/{serverId}`. `serverId` is exactly 64 lowercase
hexadecimal characters. No query, redirects, cookies or compression are allowed;
verify TLS. The signing authority is the configured URL authority (host and any
explicit port, without scheme, path or trailing slash); host spelling is canonical
lowercase. Never substitute the request's untrusted forwarded host.

The request is at most 256 UTF-8 bytes and has precisely this compact JSON order,
with no unknown/duplicate keys, whitespace or trailing bytes:

```json
{"schema":"atrinik-rendezvous-admission-v1","ticketHash":"64lowerhex"}
```

`ticketHash = SHA256(binary32(ticket))`, never SHA256 of its hexadecimal text.
For ordinary clients ticket is the existing random 32-byte attempt ticket; for
protected clients it is the resolved grant. Challenge issuance is blind and
stateless: every syntactically valid server ID receives the same success shape,
without presence, D1, room lookup or persisted challenge state. Apply a fixed
shared ingress purpose circuit before work; never create requester/IP buckets.

Response is at most 512 UTF-8 bytes, with exact compact key order:

```json
{"schema":"atrinik-rendezvous-admission-v1","challenge":"TOKEN","difficulty":24,"expiresAt":"UTCseconds"}
```

Difficulty is the integer 24, never server-selected work outside this fixed
contract. `UTCseconds` is canonical positive decimal Unix seconds, at most
253402300799. `TOKEN` has six dot-separated ASCII fields:

`v1.KEYID.IAT.TICKETHASH.NONCE.MAC`.

KEYID is `[A-Za-z0-9_-]{1,32}`. IAT is canonical positive decimal UTC seconds
with the same upper bound; expiry IAT+60 must also fit that bound. TICKETHASH and
MAC are exactly 64 lowercase hex characters; NONCE is exactly 32 lowercase hex
characters encoding 16 CSPRNG bytes. No leading zeros in IAT, extra fields,
noncanonical case or whitespace. Expiry is exactly IAT+60; response expiresAt
must equal it. Reject `now < IAT` and `now >= IAT+60` using UTC seconds.

Sign with the current source-tag key, using HMAC-SHA256. Verification permits only
configured current/previous source-tag keys and their existing greater-than-24-hour
overlap; an unknown KEYID fails closed. Key-ID lookup never supplies a key from
untrusted input. HMAC input is the following ASCII concatenation, where every
`NUL` is one literal zero byte and prefix includes everything before `.MAC`:

`atrinik-rendezvous-admission-v1 NUL authority NUL classic NUL serverId NUL prefix`.

These purpose bytes are distinct from source, replay and work tags. Compare MACs
in constant time; ticket hash and server/authority/profile are bound to the proof.

## Work and client upgrade

SOLUTION is exactly 16 lowercase hex characters. Compute SHA256 of:

`atrinik-rendezvous-work-v1 NUL complete_TOKEN NUL SOLUTION`.

Both TOKEN and SOLUTION here are their ASCII text, not decoded hex. Require the
first 24 digest bits to be zero (first three bytes exactly zero). No truncation of
TOKEN, omitted MAC or alternative solution encoding is allowed. PoW is a
computational cost gate, not an anonymous-attacker fairness guarantee.

Every client upgrade carries exactly one
`X-Atrinik-Rendezvous-Admission: TOKEN.SOLUTION` header, at most 256 ASCII bytes.
Missing admission returns the fixed 428 response before room invocation; invalid
admission returns fixed 403, without revealing a validation reason. Duplicate,
malformed and oversized headers fail closed. Public callers may not supply
internal attestation headers.

Ordinary upgrades also carry exactly one
`X-Atrinik-Rendezvous-Ticket: ticket64lowerhex`; forbid Authorization and
`X-Atrinik-Access-Client-Nonce`. Protected upgrades instead carry exactly one
`Authorization: Bearer grant64lowerhex` and
`X-Atrinik-Access-Client-Nonce: nonce64lowerhex`; ticket equals grant, and the
ordinary ticket header is forbidden. Secrets never occur in URL, query or
subprotocol. Authenticated server-role requests preserve their existing
handshake/subprotocol and forbid all client-only admission/ticket/client-nonce
headers; they do not perform client PoW.

Core validates sizes, header mode, ticket hash, HMAC, work and expiry before
room dispatch. The new internal v5 envelope attests validated 24-bit admission,
expected ticket and proof expiry, and protected grant/client nonce when present.
Room rejects internal v4 or absent client attestation; no public input can create
an attestation. Exact private envelope serialization belongs to Worker, while
these mandatory semantics are shared authority.

## Reservation, replay and authorization

Inside the room's serialized admission queue, independently check proof expiry,
server/control liveness and generation, and capacity. Protected grants are
atomically redeemed in D1 before WebSocketPair creation or acceptWebSocket: the
active/unexpired/current-revision route check and one-use redemption commit must
be one transaction. Re-read local server/control/time/generation after every
await and check liveness. A grant expiry never extends because challenge expiry
is longer.

Atomically reserve admission and claim existing purpose-separated ticket replay
tags before pair allocation. A claimed ticket stays retained for 24 hours even
on later allocation, delivery or confirmation failure; never unconsume a redeemed
grant. A different solution or challenge for the same ticket cannot allocate a
second socket, including across source-tag key rotation, hibernation or control
generations. Rejected missing/invalid/replayed proofs and invalid/expired/revoked
grants allocate zero sockets and zero admission rows.

Candidate ticket must equal the preauthorized ticket, or its exact hash if the
ordinary raw ticket is not retained. `beginClientTicket` uses the preclaimed
reservation and never claims twice. Preserve the post-hash liveness check so an
await cannot revive a dead control. Protected `access_init` / `access_ready`
frames confirm the already redeemed bound grant and client nonce only. They never
redeem, replace credentials or supply another authentication path. No candidate
is forwarded before confirmation. Existing candidate bounds and final game
access-code authentication remain unchanged.

## Native bounds and cutover

Native challenge parsers require the exact response grammar, fixed difficulty
24, token grammar, and expiresAt consistency. Solver trials are bounded by
2^27 and its deadline is `min(original_attempt_deadline, solver_start + 10s)`.
Check cancellation and deadline at least once per 4096 trials, including before
starting. Failure is terminal; retry requires a fresh grant/ticket. Do not extend
the original 15-second protected grant/attempt deadline. Benchmark representative
native costs; slower devices may hit the timeout and MUST NOT bypass work.

This is an incompatible mandatory admission-v1 cutover with no legacy fallback.
Old client to new Worker receives 428; new client to old Worker gets challenge
404/fixed unavailable and stops. Internal v4/v5 mismatch fails closed. Server
control wire frames retain their current format. No Protobuf change or legacy
schema regeneration accompanies this transport admission change.

## Independent conformance

The `rendezvous-admission-v1.json` fixtures contain synthetic keys,
identities, purpose input bytes, a solved fixed 24-bit work vector, current/previous
rotation acceptance and negative cases. Verification recomputes HMAC and work;
it never re-mines a solution. Required implementation acceptance also includes
concurrent proof admission at most once, allocation failure retaining replay,
hibernation/rotation/generation fences, invalid grant before allocation, malicious
bounds/substitution/difficulty/expiry, and native cancellation/deadline tests.
No real key, account, address, grant or server identity belongs in fixtures/logs.
