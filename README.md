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
- `gen` contains reproducible generated Go/Rust bindings and descriptors.
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
SBOM, build provenance, notices, and MIT license.

Read [CONTRIBUTING.md](CONTRIBUTING.md) and [PROVENANCE.md](PROVENANCE.md)
before proposing contract material. The cross-repository roadmap is
[atrinik/atrinik#168](https://github.com/atrinik/atrinik/issues/168).
