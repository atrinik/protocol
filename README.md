# Atrinik Game Protocol 1

This repository is the independently releasable MIT source of truth for Game
Protocol 1: Protobuf schemas, normative QUIC/framing/value specifications,
generated Go and Rust contracts, descriptors, and language-neutral fixtures.
It is a new protocol and has no classic packet-ID, C/Python, MAP2, ADS, gRPC,
or GPL compatibility path.

## Development model

Game Protocol 1 is part of Atrinik's agentic next-generation reimplementation
and improvement of Atrinik Classic. This fresh MIT-licensed Protobuf,
specification, and Go/Rust support code is developed primarily through
Codex-driven workflows under maintainer direction, review, provenance controls,
tests, and validation. Direct human-written code and specification
contributions are welcome under the same requirements; “agentic” describes the
project's primary current software-development workflow, not every line,
commit, or contributor.

The current implementation is a new independently authored contract supporting
the replacement, not a mechanical translation of Classic packet IDs or code.
That historical fact does not prohibit later exact historical reuse admitted
under the [provenance policy](PROVENANCE.md). See
[the replacement roadmap](https://github.com/atrinik/atrinik/issues/168) and
[the canonical project authorship statement](https://github.com/atrinik/atrinik/issues/331)
for the wider technical and authorship context.

The Go and Rust bindings in this repository are deterministic outputs of the
Protobuf toolchain, not generative AI output. Protocol message instances can
carry or refer to maps, quests, lore, dialogue, art, audio, and other game-world
content created by people. The schemas and deterministic bindings define how
those values are represented; they do not create or author the game world.

## Repository contract

- `proto/atrinik/game/v1` owns gameplay schemas;
  `proto/atrinik/metaserver/v2` owns the current public directory model; v1
  remains immutable historical baseline evidence.
- `spec` owns bounds, units, ordering, state, authorization, privacy, framing,
  stream, close, and compatibility rules that Protobuf cannot express.
- `gen` contains reproducible generated Go bindings and descriptors. Generated
  Rust bindings live inside `crates/atrinik-protocol/src/generated` so the
  registry package is self-contained.
- `framing`, `validation`, and `metaserver` are the small Go consumer support
  packages.
- `crates/atrinik-protocol` is the Rust consumer package.
- `fixtures` is language-neutral conformance input; negative cases must leave
  previously committed consumer state unchanged.
- `compatibility/baseline.binpb` is the M1 Buf breaking baseline. Never replace
  it to hide an incompatible change.

The current [access-token contract](spec/access-tokens.md) defines Classic
publisher v3, Game publisher/directory v2, bounded code routing, and mandatory
GP1 1.1 access-policy negotiation. Current Go consumers import
`github.com/atrinik/protocol/metaserver/v2`; Rust uses `metaserver::v2`,
`metaserver::directory_v2` and `metaserver::access`. Earlier versioned adapters
are historical contracts, not runtime fallbacks.

The current ALPN is `atrinik-game/1`. Frames use canonical unsigned LEB128
lengths and are capped at 1 MiB for gameplay/control or 4 MiB for resources.
See [the transport specification](spec/transport.md) and
[common-value specification](spec/common-values.md). Certificate-bound
metaserver publishers use the strict RFC 9421/RFC 9530 profile in
[the publisher specification](spec/metaserver-publisher.md), including exact
canonical classic and Game Protocol 1 body fixtures and the Game publisher
JSON Schema. Classic publisher v1 remains frozen with human-password semantics;
the distinct signed Classic v2 profile, schema, Go codec, and conformance
fixtures use `accessCodeRequired` for the open/access-code-protected cutover.
The replacement server directory uses the independently versioned bounded
model and canonical static JSON contract in
[the directory specification](spec/metaserver-directory.md), with generated
Go/Rust model types and byte-identical conformance fixtures.

## Toolchain and validation

The supported tools are Go 1.26.6, Rust 1.97.1, Protobuf 35.0, Buf 1.72.0,
protoc-gen-go 1.36.11, and protoc-gen-prost 0.5.0. The reusable Atrinik Linux
devcontainer supplies them. A clean clone needs no sibling checkout.

```sh
tools/generate.sh
tools/validate.sh
```

`tools/generate.sh` is the only supported generated-output command. It checks
tool versions, formats/lints schemas, regenerates both languages, and rebuilds
the descriptor. `tools/validate.sh` additionally checks the breaking baseline,
generated drift, Go vet/unit/race/fuzz compilation, Rust format/Clippy/tests/doc
tests/Windows cross-build, dependency licenses, fixtures, and release dry-run.
Cargo resolves dependency versions from `Cargo.lock`; Go uses the requirements
in `go.mod` and minimal version selection, while `go.sum` authenticates module
content. `policy/dependencies.json` records ownership and review policy for
direct dependencies without duplicating versions; validation compares it with
the manifests, checks the resolved Cargo and Go graphs for forbidden sources,
and evaluates resolved Cargo license expressions against the SPDX policy.

The aggregate required check is `Protocol validation`. Release tags create a
source/bindings/schema archive, descriptor, fixtures, checksums, CycloneDX
SBOM, build provenance, notices, and MIT license. Every release asset is a flat
file covered by the outer `SHA256SUMS`. The deterministic
`atrinik-protocol-contracts-VERSION.tar.gz` additionally contains the complete
contract layout, both nested directory fixture trees, and its own checksum
manifest. Extract that bundle to validate nested fixtures; the GitHub asset
glob cannot upload directories. A Rust crate version has
exactly one policy-owned repository release, revision, asset name, and digest
in `policy/rust-crate-release.json`. Only that owning release may include the
self-contained registry-ready `.crate`; later repository releases omit it
until a separately reviewed policy assigns a new crate version. Provenance
records both coordinates and whether the crate is included, so a repository
release never silently regenerates a published crate version from a different
Git revision.

## Rust registry publication

Registry publication follows the reviewed manual workflow described below.
The [0.2.0 candidate](#reviewed-access-token-crate-candidate) has reviewed source
and artifact pins, but remains unpublished until a separately authorized
publication succeeds. Its manifest allows only crates.io.

Crate `atrinik-protocol` version `0.1.0` is registered on crates.io with
SHA-256
`413c4da6c1b304d4a622065efe0d36c3f591041972f1a5ee76c538926f3c0b6b`.
That checksum is the reviewed asset from release `v1.4.0`, revision
`47b821a16ba955bebc79fc31e3b3bada8d74b33e`. The one-use bootstrap workflow
has been removed, its GitHub environment secret was deleted, and its API token
was revoked after the public registry checksum was independently verified.

Release `v1.4.0` is the sole owning repository release for crate `0.1.0`.
Release `v1.5.0` predates the one-owner enforcement and contains a second,
noncanonical asset with the same crate version but different revision-derived
bytes. That historical asset must never be published or substituted for the
policy digest. It is retained as immutable release history; ordinary future
repository releases omit crate `0.1.0` rather than regenerating it.

`policy/rust-crate-publishing.json` records the registered coordinates and
records future publication as `reviewed-manual-publication`. Publishing is
permanent and is never implied by merging ordinary protocol changes or by the
semantic-release workflow.

The source workflow and fail-closed policy validation are present. Environment
and Trusted Publisher setup, release-asset attachment with its complete checksum
inventory, and publication dispatch each require separate authorization; merging
this source does not authorize those operations. See
[the preparation and publication procedure](#preparing-and-publishing-the-reviewed-rust-crate).

Read [CONTRIBUTING.md](CONTRIBUTING.md) and [PROVENANCE.md](PROVENANCE.md)
before proposing contract material. The cross-repository roadmap is
[atrinik/atrinik#168](https://github.com/atrinik/atrinik/issues/168).

## Reviewed access-token crate candidate

Crate 0.2.0 was published by the separately authorized
[publication run](https://github.com/atrinik/protocol/actions/runs/38156755961).
The public API, sparse index and downloaded archive agree on the checksum below.
`policy/rust-crate-candidate.json` retains its historical preparation declaration;
`policy/rust-crate-next.json` marks the reviewed artifact `ready-for-publication`
and pins its prepared source release `v2.8.0`,
revision `a47537790f5ea8a08adf7e6dffee4fa794cf3665`, asset
`atrinik-protocol-0.2.0.crate` and SHA-256
`cda65c3c322993cfab378454fb9b1182df8a000216f4abd1170e53cdfdc3b3bb`.
The approved preparation [run](https://github.com/atrinik/protocol/actions/runs/37225929702)
packaged that clean source twice, and an isolated local reproduction matched.
The immutable published 0.1.0 policy remains unchanged. Later source releases omit these historical registry crates; they cannot
regenerate or replace the prepared bytes. Future reviewed version bumps use
the automatic publication contract below.

## Preparing and publishing the reviewed Rust crate

The manual `publish-crate.yml` workflow has explicit `prepare` and `publish`
operations. Its default is `prepare`, which requests no OIDC token and only emits
an Actions artifact. A preparation approval never authorizes publication. Inputs
for publication must match the exact reviewed tag and revision above. Both
operations run only on main in repository ID 1327106950; checkout uses the fixed
workflow-run revision, while packaging uses the separately pinned source release.
Never package activation HEAD as a substitute for the prepared source.

The one-time publication attached the exact prepared archive to
[v2.8.0](https://github.com/atrinik/protocol/releases/tag/v2.8.0), then verify its
public asset digest. The verifier refuses a missing or mismatched asset; it cannot
substitute an Actions artifact. GitHub reports the source release as mutable, so
repeated tag, provenance and asset-digest checks detect drift rather than claiming
GitHub release immutability. Attaching a new asset also requires a separately
reviewed complete checksum-inventory update; never leave an unchecked extra asset
or silently alter an existing contract/source archive.

A credential-free `verify` job reproduces the pinned source twice and validates
its release asset, public package/owner identity and registry API/sparse-index
state. Missing or conflicting evidence stops before the protected publication
job. An already published identical version is an idempotent success; different
bytes or an indeterminate registry state stop without upload.

The publication job requires `operation=publish`, that successful preflight and
the `crates-io-release` environment. The automatic environment is main-only with zero required reviewers, zero wait
time, and no registry secrets or variables. Reviewed merges and successful
checks authorize each release after one-time owner setup.
Environment and crates.io Trusted Publisher setup are separately authorized owner
operations, tracked in [github-settings#86](https://github.com/atrinik/github-settings/pull/86).
Trusted Publishing binds `atrinik/protocol`, `publish-crate.yml` and that exact
environment. Only this job grants `id-token: write`, which gives every step OIDC
request authority. It never installs a toolchain or runs Cargo, package builds,
proc macros or dependency code. After environment admission, `verify-upload` fetches
the exact public release archive as inert bytes and checks its reviewed digest,
inventory and provenance. It freshly checks the workflow checkout, pinned source
ancestry, public tag/release, complete release inventory, owner and registry state
before token exchange. It writes the same checked archive snapshot for upload.
All reproduction remains in the credential-free jobs; packaging also refuses
OIDC request authority at runtime. Only the upload step receives the short-lived
token from the pinned official action; its post step revokes the token. No long-lived registry credential is allowed.

The bounded uploader verifies the archive snapshot and public state again, then
sends those exact archive bytes using the documented
[Cargo registry publish API](https://doc.rust-lang.org/cargo/reference/registry-web-api.html#publish).
It does not invoke `cargo publish`, which would repackage the source. The request
uses a fixed HTTPS endpoint, refuses redirects, never retries an uncertain write,
and never logs the token or response body. After an attempted upload, the
credential-free verifier checks public package/owner identity and agreement
between the registry API and sparse-index checksum, including when the uploader
fails. It skips cancelled runs and skipped uploads; a matching checksum does not
turn a failed upload step into workflow success. An uncertain result requires
inspection before another separately authorized publication attempt.

This source workflow is not evidence that the environment, Trusted Publisher,
release asset or registry publication has been configured or executed. Consumers
still need the actual registry release and normal dependency-lock validation.
See the [official token action](https://github.com/rust-lang/crates-io-auth-action).


## Automatic Rust library releases

After one-time Trusted Publishing setup, a reviewed Cargo version bump publishes
without a second release approval. Repository semantic-release versions remain
independent. `policy/crate-release.json` defines the public crate allowlist,
required source checks, workflow/environment identity and historical pins.
`policy/rust-crate-candidate.json` accompanies a new source version in the same PR;
future candidates use `reviewed-automatic-publication`. No post-release policy PR
is needed to copy a checksum back into source.

The read-only source job plans the next repository version, runs the aggregate,
packages the public crate graph twice and builds the extracted archives. It emits
`crate-release.json` with actual source revision, source tag, inventory, registry
metadata, archive SHA-256 and a content checksum excluding only VCS metadata.
Changed package content with a reused Cargo version fails. Historical 0.1.0 and
0.2.0 retain their original bytes; unchanged versions are omitted from subsequent
source releases. A pending publish set forces at least a patch source release
even when ordinary commit rules would otherwise skip it.
Maintenance branches keep their ordinary source/binary releases, but their real
publication manifests contain no registry coordinates and omit registry crate
assets. Only mainline preparation may establish a crate's owning source release;
the inert finalizer independently rejects a nonempty maintenance manifest.

A separate write-enabled job reanalyzes the repository release and verifies the
complete prebuilt payload with the inert finalizer before publishing it. Source
release immutability is enabled once through `github-settings` before activation.
The finalizer uses no administration credential: the settings endpoint cannot be
read using `GITHUB_TOKEN`. The pinned GitHub release plugin creates a draft, uploads
the complete asset inventory, and only then makes it public. Every registry path
requires the resulting public release to report `immutable=true`. Semantic-release
then explicitly dispatches `publish-crate.yml` with `operation=automatic`, exact
`source_release_id`, `source_revision` and `manifest_sha256`. This avoids GitHub's
suppression of release events created with `GITHUB_TOKEN`, and crates.io's rejection
of `workflow_run` OIDC exchanges.

The registry workflow checks protected-main ancestry, the exact source workflow
and required checks, complete release checksums, safe archive inventory and
metadata. Immediately before upload it rechecks release and asset identities,
names, sizes and digests; download counters and editable release notes do not
constitute artifact drift. Its fresh upload job runs no Cargo or package code. It exchanges OIDC
immediately before sending exact verified archive bytes. Internal dependencies
publish first; API, sparse-index and downloaded checksums must agree before the
next crate proceeds. Matching existing versions are skipped, conflicting bytes
or yanked versions fail, and an uncertain PUT is followed only by bounded reads.
Hourly reconciliation dispatches one incomplete source release through these same
gates. It does not change versions, overwrite artifacts, yank crates or merge PRs.

If repository immutability is misconfigured, a source release can become public
but bootstrap and OIDC publication both fail before any registry write. Enabling
the setting does not retroactively make that release immutable. Stop automatic
reconciliation for the affected lineage and use explicit owner-reviewed recovery:
confirm that none of its crate coordinates were published, preserve its evidence,
and replace the mutable source release/tag under the corrected setting. The
automation never deletes or rewrites a release to recover. Resume only when the
exact source, checksums and immutable public release pass the ordinary gates.

Local candidate verification is credential-free:

```sh
python3 tools/crate_prepare.py check-version --base BASE_COMMIT
python3 tools/crate_prepare.py prepare --dry-run --source-tag v0.0.0 --output /tmp/crate-proof
python3 -m unittest tools.test_crate_release
```

The dry-run manifest is preparation evidence only. Live publication requires a
clean reviewed revision, complete immutable source release and configured identity.
New crate names in other families need a one-time first publication before crates.io
Trusted Publishing can be registered; this does not introduce per-version approval.

The shared `bootstrap` helper is only for these previously absent library names.
An owner runs it from an interactive terminal on a clean reviewed main revision,
with exact `--release-id`, `--revision`, `--manifest-sha256`, `--expected-owner`
and `--expected-owner-id` arguments. It verifies the same immutable source and
registry evidence, requires a typed owner confirmation, and prompts privately for
a short-lived publish-new token. Create that token in the named owner's crates.io
account; the API cannot introspect its identity. Never put it in chat, shell
arguments, environment variables or GitHub secrets. The helper checks ownership
after each confirmed first publication. Revoke the token and register Trusted
Publishing once the names exist. Established protocol names cannot use this path.
