# Atrinik protocol repository guide

## Authority and boundaries

- This fresh MIT repository is the source of truth for Game Protocol 1 and the
  shared metaserver publisher/public-directory contracts: Protobuf schemas,
  Buf policy, normative specifications, descriptors, generated Go/Rust
  contracts, and language-neutral conformance fixtures.
- Current access-token interfaces live in `spec/access-tokens.md`, metaserver
  v2 schemas/adapters and GP1 1.1 access negotiation. Retained v1 schemas and
  adapters are immutable baseline history, never a current runtime fallback.
- Reviewed Cargo version changes authorize automatic publication after protected-main
  merge, exact-revision checks and complete source-release preparation. Cargo and
  repository semantic-release versions remain independent. `policy/crate-release.json`
  owns the public allowlist, workflow identity, required checks and immutable
  historical lineage. Keep the existing 0.1.0 and 0.2.0 pins unchanged; a future
  source version updates its candidate declaration in the same reviewed change.
  Generated release manifests record actual revisions/digests, never predicted pins.
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
  `0.1.0` completed the one-use bootstrap; its workflow, secret and token stay
  retired. Version 0.2.0 is confirmed in the registry; preserve its separately
  pinned manual path and original source/archive lineage as historical evidence. Future reviewed version changes publish automatically through
  Trusted Publishing in the main-only `crates-io-release` environment, without
  required reviewers or wait timers. One-time environment/registration setup
  remains an owner operation, tracked in `github-settings`.
- Source packaging runs twice in a credential-free read-only job, with independent
  extracted-package verification. No registry token, OIDC capability or GitHub write
  credential may reach Cargo/build scripts. The write-enabled semantic-release job
  only checks and publishes the complete prebuilt inventory. It must refuse any
  difference in its independently analyzed source version or revision.
  Governance enables immutable releases before activation; the source job does
  not acquire administration credentials. If misconfigured, a mutable source
  release can be created, but every registry path must refuse it. Recovery needs
  explicit owner review and preserved evidence; never automatically delete or
  rewrite its release/tag or publish from a mutable artifact.
- Automatic dispatch selects exact source release ID, revision and manifest digest;
  these inputs are verified against immutable public source assets, protected-main
  ancestry and successful required checks. The manifest must carry a verified
  GitHub artifact attestation from the exact main `release.yml` commit and successful
  certificate-bound run attempt. Sign in a separate inert OIDC job, never the
  npm-enabled semantic-release or Cargo job. The installed `gh` verifier receives
  only a local bundle, an empty auth config and a credential-free environment.
  Every step in the protected upload job
  has OIDC capability: never run Cargo, toolchain setup, package installation or
  dependency code there. Upload only the checked byte snapshot, expose the
  short-lived token only to that step, and confirm API/index/download checksums
  before advancing dependency graphs. Failed/uncertain uploads use bounded reads;
  never blindly replay writes or silently change crate versions. Atomically reserve
  the coordinate-global `crate-upload-intent/<crate>/<version>` Git tag before any
  PUT; only explicit create acknowledgement and readback authorize that attempt.
  Existing/ambiguous intent blocks retries even when both registry views are absent.
  No automated intent deletion/reset or yank. Owner recovery preserves evidence and
  needs authoritative confirmation that no write was accepted before a new attempt.
- Each name/version has exactly one owning source release. Later source releases
  omit unchanged registry archives; content changes require reviewed version bumps.
  Maintenance source/binary releases never own registry coordinates or attach the
  selected public crate archives; preparation and inert finalization both enforce
  this boundary. Only mainline releases can introduce registry ownership.
  Keep flat downloadable assets with complete checksum coverage. Hourly reconciliation
  dispatches one unconfirmed release through the same gates; it does not bypass a
  failed check, owner bootstrap or missing registration. Registry readiness permits
  consumer update PRs, never automatic merges.
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
- `Dependency review` is nonblocking advisory feedback for PRs to `main` across
  runtime, development, and unknown dependency scopes at every severity.
  Implementing agents must read its PR comment or job summary and fix actionable
  introduced risks, or record an explicit maintainer acceptance of existing risk,
  before merge. This adds no required human approval gate; preserve all other
  validation and security checks.
