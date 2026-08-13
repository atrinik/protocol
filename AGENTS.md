# Atrinik protocol repository guide

## Authority and boundaries

- This fresh MIT repository is the source of truth for Game Protocol 1 and the
  shared metaserver publisher/public-directory contracts: Protobuf schemas,
  Buf policy, normative specifications, descriptors, generated Go/Rust
  contracts, and language-neutral conformance fixtures.
- GP1 is not the numeric C/Python registry in `atrinik/classic/protocol`. Do
  not add classic IDs, MAP2/ADS compatibility, C/Python bindings, or a dual
  protocol path here.
- Own only information crossing a network process boundary and the transport
  semantics needed to carry it. Specifications define bounds, units, ordering,
  authorization assumptions, state transitions, framing, failures,
  compatibility, and privacy—not just Protobuf syntax.
- Server owns authority/simulation; client owns session state/actions;
  content-toolkit owns authored schemas/artifacts; renderer owns scenes/GPU;
  metaserver-worker owns discovery implementation. Generated messages are
  adapters, never consumer domain models.
- Do not add runtime logic, databases, authored content, UI/rendering details,
  Go-to-Rust FFI, submodules, or general `Any`/`Struct` object graphs.

## Schema and transport contracts

- Use versioned bounded-domain packages with a narrow shared vocabulary. Every
  ID defines scope, lifetime, stability, opacity, and generation. Specify
  coordinate spaces, units, clocks, numeric ranges, overflow, presence/default,
  and unknown field/enum behavior.
- Bound strings, bytes, collections, pages, nesting, messages, transactions,
  frames, queues, and rates at Atrinik's semantic layer. Stable typed identity
  and codes must not derive from display text, enum position, DB keys, paths,
  renderer slots, or pointers.
- Model changes with request/transaction IDs, revision/generation
  preconditions, idempotency, lifecycle, and reconnect rules. Chunked snapshots
  require begin/chunk/commit plus count/size/digest and discard semantics; no
  invalid partial transaction may replace committed state.
- Specify QUIC ALPN/TLS identity, negotiation, length framing, size limits,
  stream ownership/order/reliability/deadlines/reset/close, flow control,
  backpressure, slow-peer behavior, connection limits, and graceful drain.
  Datagrams are opt-in per justified message. Framing validity never implies
  authorization.
- Servers send only viewer-authorized facts; clients send typed intent, never
  authority or executable UI/script/shader content. Keep structured error
  classes bounded and free of secrets/account-enumeration oracles.

## Generation, compatibility, and tests

- Buf configuration, schemas/specs, pinned generators, compatibility baseline,
  and deterministic descriptors are authoritative. Never hand-edit generated
  output; clean regeneration must be byte-identical and released consumers must
  build without sibling checkouts. Keep generated Rust bindings inside the
  `atrinik-protocol` crate root. Give every crate version exactly one
  policy-owned repository release, immutable revision, asset, and digest;
  later repository releases must omit that crate rather than regenerate the
  same version with different VCS metadata. Require the aggregate release
  check to build the packaged `.crate`, verify its file inventory, and prove
  that it contains no path or Git dependency.
- Reserve removed fields/values. A semantic change to units, bounds, defaults,
  identity, or state machines requires explicit compatibility review even when
  wire-compatible. Incompatible changes need an issue, consumer inventory,
  version/migration decision, coordinated artifacts, and fixtures; never reset
  or suppress breaking checks to pass.
- Maintain independent golden/negative fixtures across Go producers and Rust
  consumers. Cover limits, absence/defaults, unknowns, stale/duplicate/replay,
  ordering, authorization, cancellation, reconnect/version mismatch,
  fragmentation, truncation, invalid UTF-8, overflowing lengths, and partial
  transactions. Fuzz framing and semantic validators with bounded panic-free
  failure.
- Keep secrets/private state/server identities out of fixtures and diagnostics.

## Licensing and validation

- New schemas, specs, fixtures, generators, generated artifacts, tests, and
  infrastructure are MIT. Do not add GPL/AGPL dependencies, bundles, or
  unapproved GPL material. Independent implementation remains the default when
  exact historical reuse cannot be proven. Admit reuse only under local
  `PROVENANCE.md` and the canonical
  [`atrinik/atrinik` registry](https://github.com/atrinik/atrinik/blob/main/docs/PROVENANCE.md):
  each selected, independently separable contribution must itself fit one
  historical row's “original past Atrinik contributions solely authored”
  scope. Rows do not combine for joint, agent-generated, or inseparable work;
  later or agent-generated material needs separate contemporaneous compatible
  rights. The Classic source stays GPL-2.0-or-later; only exact recorded
  destination material receives MIT permission.
- `atrinik/atrinik#168` is the program roadmap; owning issues define product
  semantics and milestone gates. Do not copy M1-M6 schedules into this guide.
- Treat registry publication as an irreversible external operation. Version
  `0.1.0` completed the one-use bootstrap and the bootstrap workflow, secret,
  and token are retired. Future versions require crates.io Trusted Publishing
  bound to the exact repository, reviewed workflow, and protected
  `crates-io-release` environment. Pin the immutable release revision, version,
  asset, and digest; reproduce bytes before authorization; expose the exchanged
  short-lived token only to the upload step; and verify the public checksum.
  Never publish from a pull request, moving branch, dirty tree, unreviewed
  artifact, long-lived registry secret, or automatic semantic-release side
  effect. Keep publication disabled until a separate policy activation review.
- Run the aggregate contract now present:

  ```sh
  tools/validate.sh
  git diff --check
  ```

  `Protocol validation` owns format/lint, correct-baseline breaking checks,
  deterministic generation/drift, descriptors, Go/Rust builds/tests,
  cross-language fixtures, fuzz smoke, docs, and dependency/license review.
- Validate every affected producer/consumer independently. Wrapper replacement
  adapters are not available yet; do not route GP1 through classic protocol
  code. Commits/PR titles use Conventional Commits and semantic-release owns
  publication.
