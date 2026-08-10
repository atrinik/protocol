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
python3 tools/check-crate-release-policy.py
python3 -m unittest tools/test_crate_release_policy.py
jq empty \
  fixtures/framing.json \
  fixtures/metaserver-directory-v1.json \
  fixtures/metaserver-directory-v1/*.json \
  fixtures/metaserver-game-publisher-v1.json \
  fixtures/metaserver-publisher-v1.json \
  schema/metaserver-game-publisher-v1.schema.json \
  schema/metaserver-directory-v1.schema.json \
  provenance/reuse.json \
  policy/dependencies.json \
  policy/rust-crate-release.json \
  policy/rust-crate-publishing.json
test -s fixtures/metaserver-directory-v1/projection.xml

release_output=
protocol_crate_listing=
protocol_crate_extract=
protocol_crate_target=
wrong_revision_release_output=
cleanup_release_validation() {
  if [[ -n ${protocol_crate_listing} ]]; then
    rm -f -- "${protocol_crate_listing}"
  fi
  if [[ -n ${protocol_crate_extract} ]]; then
    rm -rf -- "${protocol_crate_extract}"
  fi
  if [[ -n ${protocol_crate_target} ]]; then
    rm -rf -- "${protocol_crate_target}"
  fi
  if [[ -n ${wrong_revision_release_output} ]]; then
    rm -rf -- "${wrong_revision_release_output}"
  fi
  if [[ -n ${release_output} ]]; then
    rm -rf -- "${release_output}"
  fi
}
trap cleanup_release_validation EXIT

release_output=$(mktemp -d /tmp/atrinik-protocol-release.XXXXXX)
rmdir "${release_output}"
tools/package-release.sh "${release_output}" 999.0.0-validation
test -s "${release_output}/SHA256SUMS"
(
  cd "${release_output}"
  sha256sum --check SHA256SUMS
)
protocol_crate_version=$(cargo metadata --locked --offline --no-deps \
  --format-version 1 \
  | jq -er '.packages[] | select(.name == "atrinik-protocol") | .version')
protocol_crate_asset="atrinik-protocol-${protocol_crate_version}.crate"
test "$(jq -er '.schema_version' "${release_output}/provenance.json")" = 2
jq -e --slurpfile policy policy/rust-crate-release.json '
  .rust_crate == {
    name: $policy[0].name,
    version: $policy[0].version,
    asset: $policy[0].asset,
    repository_release: $policy[0].repository_release,
    revision: $policy[0].revision,
    sha256: $policy[0].sha256,
    included: false
  }
' "${release_output}/provenance.json" >/dev/null
test ! -e "${release_output}/${protocol_crate_asset}"
if grep -Fq "  ${protocol_crate_asset}" "${release_output}/SHA256SUMS"; then
  echo "Non-owning repository release unexpectedly contains Rust crate." >&2
  exit 1
fi

test "$(jq -er '.version' policy/rust-crate-release.json)" \
  = "${protocol_crate_version}"
test "$(jq -er '.asset' policy/rust-crate-release.json)" \
  = "${protocol_crate_asset}"
protocol_crate_release=$(jq -er '.repository_release' \
  policy/rust-crate-release.json)
protocol_crate_revision=$(jq -er '.revision' policy/rust-crate-release.json)
test "$(git rev-list -n 1 "v${protocol_crate_release}")" \
  = "${protocol_crate_revision}"

if [[ $(git rev-parse HEAD) != "${protocol_crate_revision}" ]]; then
  wrong_revision_release_output=$(mktemp -d \
    /tmp/atrinik-protocol-wrong-revision-release.XXXXXX)
  rmdir "${wrong_revision_release_output}"
  if tools/package-release.sh "${wrong_revision_release_output}" \
    "${protocol_crate_release}" >/dev/null 2>&1; then
    echo "Non-owning revision emitted the policy-owned Rust crate." >&2
    exit 1
  fi
  test ! -e "${wrong_revision_release_output}/${protocol_crate_asset}"
fi

protocol_crate_target=$(mktemp -d /tmp/atrinik-protocol-crate.XXXXXX)
cargo package --locked --offline --allow-dirty \
  --manifest-path crates/atrinik-protocol/Cargo.toml \
  --target-dir "${protocol_crate_target}"
test -s "${protocol_crate_target}/package/${protocol_crate_asset}"
protocol_crate_listing=$(mktemp /tmp/atrinik-protocol-crate-files.XXXXXX)
protocol_crate_extract=$(mktemp -d /tmp/atrinik-protocol-crate-extract.XXXXXX)
tar -tzf "${protocol_crate_target}/package/${protocol_crate_asset}" \
  | sed "s#^atrinik-protocol-${protocol_crate_version}/##" \
  | LC_ALL=C sort >"${protocol_crate_listing}"
diff -u policy/rust-crate-files.txt "${protocol_crate_listing}"
tar -xzf "${protocol_crate_target}/package/${protocol_crate_asset}" \
  -C "${protocol_crate_extract}"
if ! cargo metadata --locked --offline --no-deps --format-version 1 \
  --manifest-path \
  "${protocol_crate_extract}/atrinik-protocol-${protocol_crate_version}/Cargo.toml" \
  | jq -e --arg name atrinik-protocol '
      (.packages | map(select(.name == $name))) as $packages
      | (($packages | length) == 1)
        and (all($packages[0].dependencies[];
          ((.source // "") | startswith("registry+"))))
    ' >/dev/null; then
  echo "Packaged Rust protocol contains a path or Git dependency." >&2
  exit 1
fi

git diff --check
