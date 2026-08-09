#!/usr/bin/env bash
set -euo pipefail

repository=$(git rev-parse --show-toplevel)
cd "${repository}"

tools/generate.sh
buf breaking --against compatibility/baseline.binpb
git diff --exit-code -- proto gen

test -z "$(gofmt -l framing metaserver validation)"
go mod verify
go vet ./...
go test ./...
go test -race ./...
go test -run '^$' -fuzz '^Fuzz' -fuzztime=2s ./framing
go test -run '^$' -fuzz '^FuzzPublisher$' -fuzztime=2s ./metaserver
go test -run '^$' -fuzz '^FuzzDirectoryJSON$' -fuzztime=2s ./metaserver

cargo fmt --all --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace --all-targets
cargo test --workspace --doc
cargo build --workspace --target x86_64-pc-windows-gnu

tools/check-dependencies.sh
jq empty \
  fixtures/framing.json \
  fixtures/metaserver-directory-v1.json \
  fixtures/metaserver-directory-v1/*.json \
  fixtures/metaserver-publisher-v1.json \
  schema/metaserver-directory-v1.schema.json \
  provenance/reuse.json \
  policy/dependencies.json
test -s fixtures/metaserver-directory-v1/projection.xml

release_output=$(mktemp -d /tmp/atrinik-protocol-release.XXXXXX)
rmdir "${release_output}"
tools/package-release.sh "${release_output}"
test -s "${release_output}/SHA256SUMS"
rm -rf -- "${release_output}"

git diff --check
