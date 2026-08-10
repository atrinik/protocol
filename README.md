# Atrinik Game Protocol 1

This repository is the independently releasable MIT source of truth for Game
Protocol 1: Protobuf schemas, normative QUIC/framing/value specifications,
generated Go and Rust contracts, descriptors, and language-neutral fixtures.
It is a new protocol and has no classic packet-ID, C/Python, MAP2, ADS, gRPC,
or GPL compatibility path.

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
source/bindings/schema archive, a self-contained registry-ready Rust `.crate`,
descriptor, fixtures, checksums, CycloneDX SBOM, build provenance, notices,
and MIT license. The repository release version and the pre-freeze Rust crate
version are independently explicit in `provenance.json`; a release never
silently rewrites either coordinate.

## First Rust registry publication

The one-time `Register Rust crate` workflow is deliberately fixed to release
`v1.4.0`, revision `47b821a16ba955bebc79fc31e3b3bada8d74b33e`, crate
`atrinik-protocol` version `0.1.0`, and the reviewed release-asset SHA-256. It
cannot publish a different source or version. It is manual-only and requires a
protected `crates-io-bootstrap` GitHub environment whose only secret is
`CARGO_REGISTRY_BOOTSTRAP_TOKEN`.

The bootstrap token must be created by an authorized crates.io owner, permit
new-crate creation only for this reviewed operation, and never be placed in a
repository secret or local file. Configure required environment reviewers,
approve one run only after checking the immutable coordinates above, then
revoke the broad bootstrap token and delete the environment secret immediately
after the exact registry checksum is visible. A follow-up change must remove
the bootstrap workflow and establish a crate-scoped future-release policy.
Publishing is permanent and is never implied by merging ordinary protocol
changes. The workflow builds and byte-compares the crate offline before the
secret-bearing step. The upload deliberately skips Cargo's duplicate build so
dependency build scripts never inherit the bootstrap token, unsets the token
immediately after Cargo returns, and then verifies the public registry checksum.

Read [CONTRIBUTING.md](CONTRIBUTING.md) and [PROVENANCE.md](PROVENANCE.md)
before proposing contract material. The cross-repository roadmap is
[atrinik/atrinik#168](https://github.com/atrinik/atrinik/issues/168).
