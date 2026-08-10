#!/usr/bin/env bash
set -euo pipefail

repository=$(git rev-parse --show-toplevel)
cd "${repository}"

tools/generate.sh
buf breaking --against compatibility/baseline.binpb
git diff --exit-code -- proto gen crates/atrinik-protocol/src/generated

test -z "$(gofmt -l framing metaserver validation)"
go mod verify
go vet ./...
go test ./...
go test -race ./...
go test -run '^$' -fuzz '^Fuzz' -fuzztime=2s ./framing
go test -run '^$' -fuzz '^FuzzPublisher$' -fuzztime=2s ./metaserver
go test -run '^$' -fuzz '^FuzzDirectoryJSON$' -fuzztime=2s ./metaserver
go test -run '^$' -fuzz '^FuzzGamePublishJSON$' -fuzztime=2s ./metaserver

cargo fmt --all --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace --all-targets
cargo test --workspace --doc
cargo build --workspace --target x86_64-pc-windows-gnu
cmp LICENSE crates/atrinik-protocol/LICENSE

tools/check-dependencies.sh
jq empty \
  fixtures/framing.json \
  fixtures/metaserver-directory-v1.json \
  fixtures/metaserver-directory-v1/*.json \
  fixtures/metaserver-game-publisher-v1.json \
  fixtures/metaserver-publisher-v1.json \
  schema/metaserver-game-publisher-v1.schema.json \
  schema/metaserver-directory-v1.schema.json \
  provenance/reuse.json \
  policy/dependencies.json
test -s fixtures/metaserver-directory-v1/projection.xml

release_output=$(mktemp -d /tmp/atrinik-protocol-release.XXXXXX)
rmdir "${release_output}"
tools/package-release.sh "${release_output}"
test -s "${release_output}/SHA256SUMS"
(
  cd "${release_output}"
  sha256sum --check SHA256SUMS
)
protocol_crate_version=$(cargo metadata --locked --offline --no-deps \
  --format-version 1 \
  | jq -er '.packages[] | select(.name == "atrinik-protocol") | .version')
protocol_crate_asset="atrinik-protocol-${protocol_crate_version}.crate"
test "$(jq -er '.rust_crate.version' "${release_output}/provenance.json")" \
  = "${protocol_crate_version}"
test "$(jq -er '.rust_crate.asset' "${release_output}/provenance.json")" \
  = "${protocol_crate_asset}"
test -s "${release_output}/${protocol_crate_asset}"
protocol_crate_listing=$(mktemp /tmp/atrinik-protocol-crate-files.XXXXXX)
protocol_crate_manifest=$(mktemp /tmp/atrinik-protocol-crate-manifest.XXXXXX)
tar -tzf "${release_output}/${protocol_crate_asset}" \
  | sed "s#^atrinik-protocol-${protocol_crate_version}/##" \
  | LC_ALL=C sort >"${protocol_crate_listing}"
diff -u policy/rust-crate-files.txt "${protocol_crate_listing}"
tar -xOf "${release_output}/${protocol_crate_asset}" \
  "atrinik-protocol-${protocol_crate_version}/Cargo.toml" \
  >"${protocol_crate_manifest}"
if rg -n '^[[:space:]]*(path|git)[[:space:]]*=' \
  "${protocol_crate_manifest}"; then
  echo "Packaged Rust protocol contains a path or Git dependency." >&2
  exit 1
fi
rm -f -- "${protocol_crate_listing}" "${protocol_crate_manifest}"
rm -rf -- "${release_output}"

git diff --check
