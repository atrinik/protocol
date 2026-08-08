#!/usr/bin/env bash
set -euo pipefail

repository=$(git rev-parse --show-toplevel)
cd "${repository}"

tools/generate.sh
buf breaking --against compatibility/baseline.binpb
git diff --exit-code -- proto gen

test -z "$(gofmt -l framing validation)"
go mod verify
go vet ./...
go test ./...
go test -race ./...
go test -run '^$' -fuzz '^Fuzz' -fuzztime=2s ./framing

cargo fmt --all --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace --all-targets
cargo test --workspace --doc
cargo build --workspace --target x86_64-pc-windows-gnu

tools/check-dependencies.sh
jq empty fixtures/framing.json provenance/reuse.json policy/dependencies.json

release_output=$(mktemp -d /tmp/atrinik-protocol-release.XXXXXX)
rmdir "${release_output}"
tools/package-release.sh "${release_output}"
test -s "${release_output}/SHA256SUMS"
rm -rf -- "${release_output}"

git diff --check
