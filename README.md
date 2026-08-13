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
  `proto/atrinik/metaserver/v1` owns the adjacent public directory model.
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

The current ALPN is `atrinik-game/1`. Frames use canonical unsigned LEB128
lengths and are capped at 1 MiB for gameplay/control or 4 MiB for resources.
See [the transport specification](spec/transport.md) and
[common-value specification](spec/common-values.md). Certificate-bound
metaserver publishers use the strict RFC 9421/RFC 9530 profile in
[the publisher specification](spec/metaserver-publisher.md), including exact
canonical classic and Game Protocol 1 body fixtures and the Game publisher
JSON Schema.
The replacement server directory uses the independently versioned bounded
model and canonical static JSON contract in
[the directory specification](spec/metaserver-directory.md), with generated
Go/Rust model types and byte-identical conformance fixtures.

## Toolchain and validation

The supported tools are Go 1.26.5, Rust 1.97.1, Protobuf 35.0, Buf 1.72.0,
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

The aggregate required check is `Protocol validation`. Release tags create a
source/bindings/schema archive, descriptor, fixtures, checksums, CycloneDX
SBOM, build provenance, notices, and MIT license. A Rust crate version has
exactly one policy-owned repository release, revision, asset name, and digest
in `policy/rust-crate-release.json`. Only that owning release may include the
self-contained registry-ready `.crate`; later repository releases omit it
until a separately reviewed policy assigns a new crate version. Provenance
records both coordinates and whether the crate is included, so a repository
release never silently regenerates a published crate version from a different
Git revision.

## Rust registry publication

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
keeps future publication `disabled-until-reviewed-activation`. Publishing is
permanent and is never implied by merging ordinary protocol changes or by the
semantic-release workflow.

A future crate version requires a separate reviewed activation after its
policy-owned repository release exists. The crates.io owner must configure
Trusted Publishing for GitHub repository `atrinik/protocol`, workflow filename
`publish-crate.yml`, and protected environment `crates-io-release`. That change
must add a workflow pinned to the immutable release revision and digest, grant
`id-token: write` only to its publish job, expose the exchanged short-lived
token only to the upload step, require environment review, reproduce the crate
bytes before authorization, and verify the public checksum afterward. No
long-lived crates.io token or repository secret is permitted. Activation also
updates the machine-readable policy and its fail-closed validation together.

Read [CONTRIBUTING.md](CONTRIBUTING.md) and [PROVENANCE.md](PROVENANCE.md)
before proposing contract material. The cross-repository roadmap is
[atrinik/atrinik#168](https://github.com/atrinik/atrinik/issues/168).
