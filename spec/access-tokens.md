# Access tokens v1

This normative MIT contract replaces current server access invitations and shared
join passwords. Account authentication and saved players are separate and unchanged.
Historical versioned schemas remain immutable baseline artifacts only. Current
consumers MUST NOT fall back to them. Publication/deployment is a coordinated
operator cutover; writing this contract grants no deployment or release authority.

## Values and trust

Code C is exactly 16 ASCII Crockford base32 characters from
`0123456789ABCDEFGHJKMNPQRSTVWXYZ`: encode all 80 bits of 10 CSPRNG bytes without
padding. No UUID truncation or random fallback. User input may trim outer ASCII
whitespace and uppercase ASCII before validation; no aliases, separators or Unicode.
This deliberately provides 80-bit entropy, not 128 bits. Sixteen base64url characters
would supply 96 bits but require case-sensitive entry. Hash theft permits offline
search; online rate limits do not prevent that offline search.

With a literal one-byte NUL at each domain terminator:

- R = SHA256(UTF8("atrinik-access-route-v1\0") || C).
- I = SHA256(UTF8("atrinik-access-index-v1\0") || R).
- V = SHA256(UTF8("atrinik-access-join-v1\0") || serverId32 || C).

All digests are 32 binary bytes, 64 lowercase hex in JSON. Admission compares fixed-width V values in constant time and returns one public unavailable result for unknown, expired, revoked or incorrect codes. Servers persist I/V,
metaserver persists I only; neither persists C/R. C is sent only inside a pinned
server TLS/QUIC connection. R is sent only in the HTTPS resolve body. No secret in
URLs, argv, environment, logs, chat history, metrics or public listings. Deliberate
operator display/copy/capture of the one-time code is supported; applications exclude
it from automatic diagnostics/capture and minimize/cleanse secret-buffer lifetimes,
without promising impossible protection against arbitrary process-memory capture.

Profiles on all new access routes are exactly `classic` or `game`. serverId is
SHA256 of the exact DER leaf certificate. Certificate JSON is canonical padded
base64 of 1..2048 DER bytes, P-256, validated through existing publisher rules.
GP1's SPKI pin is separately derived from that same DER certificate; never compare
it directly to a DER-leaf digest. The trusted HTTPS metaserver is first-contact
routing authority; previously pinned identity changes stop before C is sent. A
code alone cannot protect first contact against a malicious trusted directory.

`accessRequired` is configured policy, independent of `public`, token count,
expiry, availability and store health. Open public servers require no token.
Public protected servers retain their explicit public endpoint if configured;
private protected servers are resolvable only with R, never publicly listed.
Private open servers allow configured direct connections only. Token possession
confers no account/operator rights.

## Publisher and directory boundary

Classic publisher v3 uses `/v3/classic/servers/{serverId}/publish`, signature tag
`atrinik-classic-publish-v3`, schema of the same name. Canonical key order:
`schema,serverId,certificate,name,playersCount,version,textComment,public,accessRequired[,hostname,port]`.
Other Classic bounds remain unchanged. Its directory schema is
`atrinik-classic-directory-v6`, protocol 6, `accessRequired` / XML `AccessRequired`.
Game publisher v2 uses `/v2/servers/{serverId}/publish`, tag/schema
`atrinik-game-publish-v2`; replace `passwordRequired` with `accessRequired` in the
same canonical key position. Game directory is `atrinik-game-directory-v2`, using
`atrinik.metaserver.v2` messages. Retain immutable v1 definitions only as history;
new v2 reserves field 13 and name `password_required`, keeps endpoint field 14,
and assigns `access_required` field 15. Current APIs/parsers use v2 only.

Minimal authenticated private presence additionally retains the last validated
DER certificate, name, optional explicit endpoint and configured policy for
resolve; these are owner-private, never projected by a public builder unless a
current `public:true` publisher body authorizes that projection. Presence expiry
uses existing publisher freshness, not token expiry. No token metadata joins
public directory bodies, R2 objects, indexes or metrics.

## Route management: exact envelopes

POST `/v1/access/servers/{profile}/{serverId}/routes` with existing RFC9421
publisher covered components, header `Atrinik-Publish-Sequence`, P-256 signature,
tag `atrinik-access-routes-v1`. Body maximum 4096 UTF-8 bytes, canonical compact
JSON, no duplicate/unknown keys, no redirects. Exact key order for all operations:

`schema,profile,serverId,certificate,operation,requestId,tokenId,tokenRevision,index,reservationId,expiresAt`.

Schema is `atrinik-access-route-v1`; operation is `reserve`, `activate`, `revoke`
or `result`. requestId/tokenId are 32 lowercase hex (16 independently random bytes).
tokenRevision is canonical unsigned-64 decimal string greater than zero; index
is I. reservationId is null for initial reserve or a 32-lowercase-hex handle for
activate/revoke; revoke may use null to establish a terminal deny before reserve.
expiresAt is null (never) or canonical positive UTC-seconds decimal text, at most 253402300799 (9999-12-31T23:59:59Z).
For `result`, requestId identifies the original operation and other fields exactly
repeat it except operation=result; the server looks up the stored canonical request
payload hash and returns its outcome, never creates a registration. Results are
owner-authenticated reads and consume a fresh signature sequence like mutations.

Authentication/signature/clock checks happen before idempotency. All access and
publisher operations share the authenticated profile/server replay lineage. A
freshly signed retry MUST use a new reserved monotonic sequence and nonce with the
same requestId and identical canonical operation payload. After authenticating and
charging bounded request cost, compare requestId's immutable SHA256(body) before
state mutation. Identical retries return stored outcome; different payload conflicts.
An old signature/sequence is rejected even when requestId exists. Explicit `result` compares every stored canonical field other than operation byte-for-byte after canonical decoding: schema, profile, serverId, certificate, requestId, tokenId, tokenRevision, index, reservationId and expiresAt must equal the original request. It is the only permitted operation-name difference; lookup never trusts an independently claimed original operation. Never use HTTP replay as business idempotency.

Success canonical response order:
`schema,requestId,outcome,reservationId,reservationExpiresAt,tokenRevision`.
Schema `atrinik-access-route-result-v1`, maximum 1024 bytes. Outcome is `reserved`,
`active`, `revoked`, `conflict`, `expired`, `not_found`, `unavailable`.
reservationExpiresAt is canonical decimal UTC seconds text or null. A reservation
handle is 16 CSPRNG bytes and is bound to owner, I, tokenId, revision and immutable
expiry; deadline is reserve commit time +60 seconds. Reserve never activates.
Activate requires that exact still-live handle and tuple; it MUST NOT upsert an
absent, expired, revoked or mismatched reservation. Revoked is terminal for a tokenId:
no larger revision may resurrect it. Reissuing requires new tokenId, C and I.
Revoke stores the maximum observed revision, invalidates all associated pending
grants and prevents delayed reserve/activate for older or later revisions from
reviving that identity. Delayed requests never reverse a tombstone.

Global I uniqueness is serialized in D1 across profiles/servers. Each server has
at most 1024 active/pending registrations and at most 4096 total retained records
including terminal tombstones. Environment totals are at most65536 route records and65536 ordinary idempotency outcome rows; refuse growth before either cap. Existing live publisher identity limits also apply. Revocation of a nonterminal retained record must remain possible at capacity: atomically enforce `retainedOutcomeRows + nonterminalRouteRows <=4096` per server and `<=65536` globally on EVERY transaction allocating an outcome, including absent-row revoke and distinct no-effect terminal revoke; those operations cannot consume slots reserved for pending/active routes. This reserves one outcome slot for each pending/active route. First revoke turns that route terminal and inserts its outcome in the same transaction, leaving the sum unchanged. Identical retry/result allocates nothing. A later distinct no-effect revoke of an already-terminal record may return unavailable when no outcome slot remains, without weakening its existing deny. Never evict replay protection to make room. An absent-row revoke at capacity returns unavailable; local revocation remains enforced and its bounded durable outbox retries after capacity becomes available. Issuance refuses before either cap is exceeded.
Tombstones and idempotency outcomes last 90 days; expired pending reservations
remain terminal during that horizon. At a retention boundary expire outcomes and
corresponding terminal rows transactionally. Never intentionally reuse codes/IDs.

The server stages V/I durably, reserves and activates remotely, then atomically
commits its local active record plus mutation receipt before responding once with
C. Failure never publishes a success receipt. Recover pending/indeterminate local
records by owner-authenticated status/revoke; never revoke all active codes on
restart merely because one-time delivery acknowledgment is unknowable. Lost
successful response means `already_committed_secret_unavailable`, then explicit
revoke/reissue. C is never reconstructed or retained for idempotent re-display.

## Resolve and rendezvous

POST `/v1/access/resolve`, request maximum512 bytes, strict exact-key-order object:
`{"schema":"atrinik-access-resolve-v1","routeCapability":"Rhex","clientNonce":"64lowerhex"}`.
No cookies, redirects or caching. Hash R to I; require active/unexpired route,
protected policy, fresh authenticated presence and a live control generation.
Unknown/revoked/expired/inactive/unavailable targets all return the same fixed
404 `{"error":{"code":"access_unavailable"}}` with `Cache-Control:no-store`.
Rate limiting uses fixed bounded429, does not distinguish token existence, and
applies cheap ingress limits before lookup. No per-guess persisted rows.

Success maximum8192 bytes, canonical key order:
`schema,profile,serverId,certificate,name,accessRequired,generation,clientNonce,grant,expiresAt[,endpoint]`.
Schema `atrinik-access-resolved-v1`; profile classic/game; certificate as above;
name uses publisher's existing bound; accessRequired is exactly true; generation
is existing 64-lowercase-hex presence generation; clientNonce echoes input; grant
is fresh32random bytes/64lowerhex; expiresAt is positive decimal UTC-seconds text
at most15seconds ahead. Optional endpoint has canonical `hostname,port` fields
using existing DNS/port validation. Neither I nor tokenId/revision appears.

Client opens WSS `/v1/access/rendezvous/{profile}/{serverId}` with subprotocol
`atrinik-access-rendezvous-v1`, then sends exactly one frame within two seconds:
`{"type":"access_init","version":1,"grant":"64lowerhex","client_nonce":"64lowerhex"}`.
This auth frame maximum256bytes, validUTF8, exact keys/order. No candidate before
acceptance. Success server frame is `{"type":"access_ready","version":1}`;
failure closes with existing fixed authorization-failure behavior. Subsequent
candidate/completion frames retain existing bounded ticket semantics: use grant
as the attempt's existing64hex ticket, but never put grant in URL/query/protocol.
The authenticated server control connection uses this same new subprotocol on
existing server-role rendezvous route, with profile-aware room dispatch. Game
transport can remain circuit-disabled until a real consumer exists.

Each grant binds profile/server/presence generation/I/token revision/client nonce,
expires within15seconds, and permits one client socket. Global route authority
serializes revoke and grant redemption through one D1 transaction: conditional
active/unexpired/current-revision route check AND insertion/transition of a
one-use redemption record are atomic. A cached D1 read then DO consumption is
insufficient. DO admits only the committed redemption receipt bound to its current
server-control generation/socket and closes pending attempts on revoke events;
no candidates forward if revocation won before redemption. If redemption won first,
revocation still closes pending routing and independent game admission denies C.
DO delivery failure does not unconsume a grant; client obtains a new grant.
D1/DO SQLite replay and ordinary tables never store raw grants/candidates; use purpose-separated replay HMAC aliases as current privacy policy requires. The existing live hibernation attachment may retain the raw one-use grant as its routing ticket for at most15seconds, bounded by socket limits and cleared on terminal teardown; this narrow transient persistence exception never permits raw access codes/R. Pending grant records cap at32/server and32768/environment; prune expired/consumed records in bounded batches before allocation and fail closed at capacity. Cross-store writes never claim
atomicity they do not provide.

Initial ceilings: resolve30/source/minute and60eligible/server/minute; route
mutations64/authenticated-server/hour burst16; existing stricter WAF/transport
limits still apply. At most16 active client attempts/server; one15second deadline,
existing frame/byte/candidate budgets and replay retention. Unknown codes cannot
drain a claimed server budget. Rotating source HMAC tags, never raw IP persistence.

## Game connection state

Classic uses a coordinated new negotiated revision. ACCESS_AUTH/ACCESS_ADMIN, ACCESS_RESULT/ACCESS_ADMIN_RESULT/ACCESS_POLICY identities and numeric allocations belong exclusively to the Classic schema owner; this MIT specification describes shared semantics, not that numeric registry.
After authenticated encrypted VERSION, server always sends exactly one POLICY
payload `[1,mode]` where mode0=open/1=protected. Client waits even for direct
connections. AUTH payload is `[1] || C` (17 bytes), once only in protected state.
RESULT is `[1,status]` status0=accepted/1=unavailable. Malformed/duplicate/unknown/
out-of-order messages fail before partial state. No setup/account/resource/gameplay
before policy and, when protected, acceptance. Retire the old join-password subtype without reuse in its owning registry. Never send C on plaintext or0-RTT. Operator request/result packets
are version1 plus strict adminJSON, bounded1024/32768 bytes, independently authorized.

GP1 stays package `atrinik.game.v1`, minimum version1.1, ALPN atrinik-game/1.
Every new peer MUST offer Capability ACCESS_TOKENS_V1=4; unsupported peers reject
before secrets. ServerHello adds access_policy field9 with AccessPolicy enum
UNSPECIFIED0, OPEN1, PROTECTED2; unspecified/unknown is invalid, never open.
AccessAuth is code bytes field1 (exact16canonicalASCII) plus SessionId field2 equal
to negotiated session; AccessResult status field1 enum UNSPECIFIED0, ACCEPTED1,
UNAVAILABLE2. ControlEnvelope adds access_auth field8 and access_result field9.
Protected auth follows ServerHello once before other authorized streams. Open
connections skip access auth; unsolicited credentials fail. Reconnect has a new
session and never replays auth. Existing TLS/QUIC transcript security, disabled
0-RTT, frame sequence, lengths and bounded failures remain normative.

## Local administration, persistence and use

In-game /access operations require explicit root-managed authenticated-account
allowlist/capability `access-token-admin`, never default OP/group inheritance.
Root/startup-only policy/store/allowlist config cannot be read/mutated through
ordinary OP `/config`, permission grants, aliases or scripting escape routes.
Use the same checked serialized store API for in-game and peer-authenticated local
admin. Bootstrap offline initializes store/allowlist only; a usable code is issued
through the locked-online authenticated control path. Never temporarily open a
server or create a default code.

A token's authorization revision changes only on management changes; last-use/audit
updates advance a separate durable commit sequence, not token authorization or
global management-CAS revision. Concurrent admission/revoke serialize at store
commit; revoked/expired sessions fence further gameplay within one second and
perform ordinary checked save/disconnect, never discard player state. Successful
admission and actual last-use commit atomically before exposing authorization.
Lists/history/provisioning/offline checks never invent use events. Default expiry
is null; no automatic rotation/renewal. A valid empty/fully expired protected store
is locked, not corrupt and not an updater failure.

Root-managed allowlist files may grant read access to the dedicated service group (root-owned0440/0640, no group-write/other access); server UID must not gain write authority. Native admin uses existing root-only UID/peer identity boundary. Request is one
`ATRINIK-ADMIN/1 ACCESS <compactJSON>\n`, write-half-close, maximum1024bytes. JSON
schema `atrinik-access-admin-v1`, operation issue/list/history/revoke/remove/status/
result, requestId32hex, operation-specific strict keys. Mutations require global
management `expectedRevision` unsigned64decimal text; issue adds label1..128UTF8
no controls and optional expiresAt decimaltext; revoke/remove/history add tokenId;
list adds optional cursor ASCII<=128, revision and limit1..64; result adds
 targetRequestId. Unknown/duplicate keys or trailing bytes fail.
Response `ATRINIK-ADMIN/1 ACCESS <N>\n` then exactly N JSON bytes then EOF;
N canonical1..32768. Exact envelope fields are schema, operation, requestId, outcome, revision and result. Successful status uses outcome=committed; initialized outer revision matches result revision, absent_open outer revision is null; pendingRouteSync is integer0..32. These bind the response; no secret except initial successful issue's private code field. Capability
is `access-tokens-v1`; old shutdown capabilities remain a bounded unordered set.

Status is a tagged union. Initialized:
`{"state":"initialized","schemaVersion":1,"serverIdentity":"64hex","policy":"open|protected","integrity":"ok|failed","durability":"ok|indeterminate","revision":"u64","pendingRouteSync":0}`.
Absent open:
`{"state":"absent_open","schemaVersion":1,"serverIdentity":"64hex","policy":"open"}`.
Absent protected is failure, never fabricated initialized state. No extra fields.
Provide the identical status via a native read-only offline command with exclusive
state lock and verified stopped server, no networking/startup/issuance. Updater
uses that path when stopped and live root socket when running; it never checks
individual expiry/last-use or renews codes. Backup includes token/audit/receipts/
route outbox in the complete stopped-state cohort, preserving identity and players.
