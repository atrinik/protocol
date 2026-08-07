# Atrinik Game Protocol 1 repository guide

These instructions apply to the entire `atrinik/protocol` repository.

## Purpose and authority

- This fresh MIT repository is the source of truth for Atrinik Game Protocol 1:
  Protobuf schemas, Buf policy, normative specifications, descriptor sets,
  generated Go/Rust contracts, and language-neutral conformance fixtures.
- Game Protocol 1 is a new contract, not an extension or source port of the
  numeric C/Python registry in `atrinik/legacy-protocol`. Do not add classic
  packet IDs, MAP2/ADS compatibility, C/Python bindings, or a dual protocol
  path.
- Use [atrinik/atrinik#168](https://github.com/atrinik/atrinik/issues/168) as
  the cross-repository roadmap. The issue owning a contract is authoritative
  for its product semantics; protocol work does not reopen established
  gameplay/content decisions.

## Scope boundaries

- Own only information that crosses a network process boundary and the
  transport semantics required to carry it. Protobuf syntax alone is not the
  whole contract: specifications must define bounds, units, ordering,
  authorization assumptions, state transitions, framing, failure behavior,
  compatibility, and privacy.
- `atrinik/server` owns authority, simulation, domain commands, persistence,
  and viewer-specific authorization. `atrinik/client` owns session/application
  state and actions. Generated messages are adapters at those boundaries, not
  either consumer's internal domain model.
- `atrinik/content-toolkit` owns authored schemas, parsing, catalogs, compiled
  artifact formats, and content validation. This repository may define bounded
  content/resource identities, digests, manifests, and delivery messages, but
  not a generic content model or compiler schema.
- `atrinik/renderer` owns renderer-neutral scenes and GPU/resource APIs. Do not
  expose renderer handles, shaders, editor documents, UI layouts, or GPU
  implementation details over the wire.
- `atrinik/metaserver-worker` owns discovery-service implementation. This
  repository owns only its versioned registration/discovery/rendezvous
  contract where specified.
- Do not add server runtime logic, database models, content files, transport
  implementations, application UI behavior, Go-to-Rust FFI, Git submodules, or
  a general-purpose `Any`/`Struct` object graph as a shortcut around a domain
  schema.

## Schema and specification rules

- Use versioned packages such as `atrinik.game.v1`, organized by bounded domain
  contracts with a narrow shared value vocabulary. Avoid a monolithic message
  file and duplicated near-equivalent common types.
- Give every ID an explicit scope, lifetime, stability, opacity, and generation
  rule. Specify coordinate spaces, units, clock origins, numeric ranges,
  rounding/overflow, presence/default semantics, and unknown enum/field
  behavior.
- Bound every string, bytes value, collection, page, nesting level, message,
  transaction, frame, queue interaction, and rate at Atrinik's semantic layer;
  generated parser limits are not sufficient. Specify rejection and recovery
  behavior for every bound.
- Use stable typed identities and semantic codes. Never infer identity or
  actions from display text, localized prose, enum position, database keys,
  filesystem paths, renderer slots, or implementation pointers.
- Model requests and changes with request/transaction IDs, revision or
  generation preconditions, explicit lifecycle states, idempotency, and
  reconnect/resynchronization rules. Messages that represent one logical
  change must be validated transactionally before consumers replace live
  state.
- Snapshots split across frames/messages require explicit begin/chunk/commit,
  expected count/size/digest, temporary validation, timeout/cancel, and discard
  rules. A truncation, duplicate, stale revision, invalid reference, or missing
  commit must leave the previously committed state intact.
- Server messages carry only viewer-authorized facts. Clients submit typed
  intent, never authority, handler names, arbitrary executable UI/script/shader
  content, or trusted outcomes.
- Do not use raw Protobuf serialization as a canonical content/state hash.
  Define a separate canonicalization contract when a digest needs stable bytes.

## QUIC and framing contracts

- Normatively specify ALPN, TLS/server identity, protocol/capability
  negotiation, length-delimited framing, exact prefix encoding, maximum frame,
  message, and decompressed sizes, stream ownership/roles, ordering,
  reliability, deadlines, cancellation, reset, priority, and close/error codes.
- Define bounded flow control, queue/backpressure, slow-peer behavior,
  connection/stream limits, keepalive/idle, reconnect/resume, graceful drain,
  and bulk-resource isolation. Transport implementations cannot invent these
  independently.
- QUIC datagrams are opt-in per explicitly justified message. No contract uses
  datagrams by default, and gRPC is not required in the gameplay path.
- Separate structured transport, authentication, protocol, content/resource,
  request-validation, and internal error classes without leaking secrets or
  creating an account-enumeration oracle.
- Framing validity never implies authorization or state validity. Go and Rust
  consumers must apply contextual validation before committing internal state.

## Generation, compatibility, and releases

- Buf module/configuration, checked-in schemas and specifications, pinned
  generator configuration, compatibility baseline, and deterministic
  descriptor output are authoritative. Never hand-edit generated Go/Rust
  outputs; change the source and regenerate all owned artifacts.
- Pin generators and dependencies. A clean regeneration must be byte-for-byte
  reproducible, and released Go/Rust consumers must build without sibling
  repository source checkouts.
- Never reuse removed field numbers, field names, or stable enum values; reserve
  them. Do not change a field's meaning, unit, bound, default, identity scope,
  or state-machine role under an unchanged contract merely because Buf sees a
  wire-compatible schema.
- Every incompatible change needs an explicit issue, consumer inventory,
  migration and version decision, coordinated Go/Rust updates, fixtures/spec
  changes, and compatibility review. Do not suppress or reset a breaking check
  to make a change pass. Use the approved major/new-package process once
  established.
- Additive changes still require bounded unknown-field/enum behavior and mixed
  version tests. A new required capability must fail during negotiation before
  credentials or gameplay state are exchanged.
- [protocol#13](https://github.com/atrinik/protocol/issues/13) freezes the Game
  Protocol 1 compatibility/release policy only after the M3 playable slice
  proves the contract. Do not declare an earlier informal freeze or postpone
  the M1 breaking baseline.
- Releases include schema source, normative specifications, descriptors,
  generated packages/artifacts, conformance fixtures, checksums, compatibility
  manifest, notices, SBOM, and provenance.

## Fixtures, tests, and security

- Maintain language-neutral golden byte/descriptor fixtures and independently
  authored negative fixtures. Go producers and Rust consumers must agree on
  bytes and semantics without sharing implementation logic.
- For each message/state machine, cover minimum, maximum, empty, unknown,
  absent/default, stale, duplicate, replayed, reordered where permitted,
  out-of-order, unauthorized, cancelled, reconnect, and version-mismatch cases.
- Test fragmentation/coalescing and truncation at every field or frame boundary.
  Exercise count/digest mismatch, oversized lengths, overflowing varints,
  invalid UTF-8 where applicable, excessive nesting, impossible references,
  unknown required capabilities, and partial transactions.
- Fuzz both framing and semantic validators. Negative/fuzz failures must be
  bounded, deterministic, panic-free, and leave prior consumer state intact.
  Compile and smoke-run fuzz targets in routine validation; run longer campaigns
  on a documented cadence.
- Keep credentials, tokens, private player state, server identities, and other
  secrets out of fixtures, debug formatting, generated snapshots, and logs.
  Include redaction and disclosure-boundary tests.
- Cross-language conformance issue
  [protocol#11](https://github.com/atrinik/protocol/issues/11) is a continuing
  gate for domain issues, not cleanup after schemas are considered complete.

## Validation contract

- Once [protocol#1](https://github.com/atrinik/protocol/issues/1) bootstraps
  Buf, generators, language consumers, and component scripts, the documented
  aggregate validation must cover format/lint, breaking comparison against the
  correct baseline, deterministic generation and drift, descriptor checks,
  Go/Rust builds and tests, cross-language golden/negative/state-machine tests,
  fuzz-target compilation/smoke tests, docs links, and dependency/license
  review. CI's eventual required aggregate check is `Protocol validation`.
- Today this seed repository contains only its README and MIT license; Buf
  configuration, schemas, generated packages, and validation scripts do not
  yet exist. For guidance-only changes, inspect the Markdown and run
  `git diff --check`. Do not report Buf, Go, Rust, wrapper build, or integration
  success until #1 provides those commands and files.
- After bootstrap, use the component-owned documented command rather than
  inventing ad hoc generator invocations. For cross-repository changes, use the
  wrapper's exact profile and build commands once its fresh component contracts
  are implemented, and validate every affected Go/Rust producer and consumer.

## Milestone priorities

- **M1 — Clean-room foundations:** bootstrap reproducible Protobuf/Buf and
  generated Go/Rust packages (#1), specify QUIC/framing/stream/error behavior
  (#2), and define common bounded value types (#3). These can develop in
  parallel behind reviewed package and dependency decisions, but all three
  gate stable domain contracts.
- **M2 — Contracts, content, and headless world:** build domain contracts for
  sessions, ordered scenes, actions/items, actors/combat, structured
  dialogue/quests, social/commerce, resource delivery, and metaserver
  discovery (#4–#10 and #12). Develop domains in parallel after common types
  and transport, continuously extending cross-language conformance (#11).
- **M3 — First playable replacement:** close compatibility and release policy
  (#13) only after the complete Go/Rust playable slice interoperates against
  released artifacts and fixtures.
- **M4–M6:** there are currently no separate protocol implementation issues in
  these milestones. Preserve the released compatibility contract while editor,
  migration, hardening, and cutover owners advance. Any genuinely new network
  contract requires its own owning issue, milestone, producer/consumer plan,
  bounds, fixtures, and version decision; do not hide feature work inside a
  compatibility fix.

Milestones are dependency gates rather than independent repository schedules.
Prototypes may begin early, but a phase closes only when its cross-repository
Go/Rust consumers and shared fixtures meet the roadmap exit criteria.

## Licensing, provenance, and delivery

- New schemas, specifications, fixtures, generators, generated artifacts, tests,
  and repository infrastructure are MIT. Do not copy, adapt, or mechanically
  translate GPL legacy schemas, generators, code, tests, or packet tables.
- Historical reuse is allowed only under the exhaustive approved-grantor
  registry and proof rules in the current `atrinik/atrinik` root `AGENTS.md`.
  A complete, non-shallow history audit must follow renames and moves, prove
  sole original authorship by an approved grantor, resolve historical
  identities, and exclude embedded third-party or conflicting material.
  Mixed, incomplete, or uncertain evidence fails closed.
- Record eligible reuse in the destination pull request or a committed
  provenance manifest: exact source repository/path/revision and full history,
  identity evidence, destination, transformation, third-party review,
  applicable grantor, and the exact wrapper commit containing the registry
  entry. A grant never blanket-relicenses a repository, file, content pack, or
  generated output.
- Every contract change names its authoritative schema/spec, producers,
  consumers, compatibility decision, bounds, failure behavior, and fixtures.
  Update all affected artifacts together; do not leave a schema-only contract
  with consumers guessing semantics.
- Use Conventional Commits for commits and pull-request titles. Update the
  wrapper supply-chain inventory whenever toolchains, generators, packages,
  Actions, images, licenses, or validation paths change. Do not commit secrets
  or confidential/unreleased project information.
