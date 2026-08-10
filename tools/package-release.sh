#!/usr/bin/env bash
set -euo pipefail

repository=$(git rev-parse --show-toplevel)
cd "${repository}"

output=${1:-dist}
version=${2:-$(git describe --tags --always --dirty)}
if [[ ! ${version} =~ ^[0-9A-Za-z][0-9A-Za-z.+-]*$ ]]; then
  echo "Invalid release version: ${version}" >&2
  exit 2
fi
if [[ -e ${output} ]]; then
  echo "Release output already exists: ${output}" >&2
  exit 1
fi
install -d "${output}"

crate_version=$(cargo metadata --locked --offline --no-deps --format-version 1 \
  | jq -er '.packages[] | select(.name == "atrinik-protocol") | .version')
crate_asset="atrinik-protocol-${crate_version}.crate"
crate_target=$(mktemp -d /tmp/atrinik-protocol-crate.XXXXXX)
trap 'rm -rf -- "${crate_target}"' EXIT
cargo package --locked --offline --allow-dirty \
  --manifest-path crates/atrinik-protocol/Cargo.toml \
  --target-dir "${crate_target}"
cp "${crate_target}/package/${crate_asset}" "${output}/${crate_asset}"

archive="atrinik-protocol-${version}.tar.gz"
git archive --format=tar --prefix="atrinik-protocol-${version}/" HEAD \
  | gzip -n >"${output}/${archive}"
cp gen/descriptor/atrinik-game-v1.binpb "${output}/"
cp fixtures/framing.json fixtures/metaserver-directory-v1.json \
  fixtures/metaserver-game-publisher-v1.json \
  fixtures/metaserver-publisher-v1.json \
  schema/metaserver-directory-v1.schema.json \
  schema/metaserver-game-publisher-v1.schema.json \
  spec/metaserver-directory.md spec/metaserver-publisher.md \
  THIRD_PARTY_NOTICES.md LICENSE "${output}/"
cp -R fixtures/metaserver-directory-v1 "${output}/"

SYFT_CHECK_FOR_APP_UPDATE=false syft dir:. \
  --source-name atrinik-protocol --source-version "${version}" \
  --output "cyclonedx-json=${output}/sbom.cdx.json"

jq -n \
  --arg version "${version}" \
  --arg crate_version "${crate_version}" \
  --arg crate_asset "${crate_asset}" \
  --arg revision "$(git rev-parse HEAD)" \
  --arg go "$(go version)" \
  --arg rust "$(rustc --version)" \
  --arg buf "$(buf --version)" \
  --arg protoc "$(protoc --version)" \
  '{
    schema_version: 1,
    version: $version,
    revision: $revision,
    rust_crate: {name: "atrinik-protocol", version: $crate_version,
      asset: $crate_asset},
    tools: {go: $go, rust: $rust, buf: $buf, protoc: $protoc}
  }' >"${output}/provenance.json"

(
  cd "${output}"
  mapfile -t directory_fixtures < <(
    find metaserver-directory-v1 -type f -print | LC_ALL=C sort
  )
  sha256sum "${archive}" "${crate_asset}" atrinik-game-v1.binpb framing.json \
    metaserver-directory-v1.json metaserver-game-publisher-v1.json \
    metaserver-publisher-v1.json metaserver-directory-v1.schema.json \
    metaserver-game-publisher-v1.schema.json metaserver-directory.md \
    metaserver-publisher.md "${directory_fixtures[@]}" sbom.cdx.json \
    provenance.json THIRD_PARTY_NOTICES.md LICENSE \
    >SHA256SUMS
)
