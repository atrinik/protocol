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

crate_policy=policy/rust-crate-release.json
jq -e '
  .schema_version == 1
    and (.name | type == "string" and length > 0)
    and (.version | test("^[0-9]+\\.[0-9]+\\.[0-9]+$"))
    and (.repository_release | test("^[0-9]+\\.[0-9]+\\.[0-9]+$"))
    and (.revision | test("^[0-9a-f]{40}$"))
    and (.asset | test("^[A-Za-z0-9_.-]+\\.crate$"))
    and (.sha256 | test("^[0-9a-f]{64}$"))
' "${crate_policy}" >/dev/null
crate_name=$(jq -er '.name' "${crate_policy}")
crate_version=$(jq -er '.version' "${crate_policy}")
crate_repository_release=$(jq -er '.repository_release' "${crate_policy}")
crate_revision=$(jq -er '.revision' "${crate_policy}")
crate_asset=$(jq -er '.asset' "${crate_policy}")
crate_sha256=$(jq -er '.sha256' "${crate_policy}")
test "${crate_asset}" = "${crate_name}-${crate_version}.crate"

metadata_crate_version=$(cargo metadata --locked --offline --no-deps \
  --format-version 1 \
  | jq -er --arg name "${crate_name}" \
    '.packages[] | select(.name == $name) | .version')
test "${metadata_crate_version}" = "${crate_version}"

crate_included=false
crate_target=
cleanup_crate_target() {
  if [[ -n ${crate_target} ]]; then
    rm -rf -- "${crate_target}"
  fi
}
trap cleanup_crate_target EXIT
if [[ ${version#v} == "${crate_repository_release}" ]]; then
  test "$(git rev-parse HEAD)" = "${crate_revision}"
  test "$(git rev-list -n 1 "v${crate_repository_release}")" \
    = "${crate_revision}"
  crate_target=$(mktemp -d /tmp/atrinik-protocol-crate.XXXXXX)
  cargo package --locked --offline --allow-dirty \
    --manifest-path crates/atrinik-protocol/Cargo.toml \
    --target-dir "${crate_target}"
  test "$(sha256sum "${crate_target}/package/${crate_asset}" \
    | cut -d ' ' -f 1)" = "${crate_sha256}"
  cp "${crate_target}/package/${crate_asset}" "${output}/${crate_asset}"
  crate_included=true
fi

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
  --arg crate_name "${crate_name}" \
  --arg crate_version "${crate_version}" \
  --arg crate_asset "${crate_asset}" \
  --arg crate_repository_release "${crate_repository_release}" \
  --arg crate_revision "${crate_revision}" \
  --arg crate_sha256 "${crate_sha256}" \
  --argjson crate_included "${crate_included}" \
  --arg revision "$(git rev-parse HEAD)" \
  --arg go "$(go version)" \
  --arg rust "$(rustc --version)" \
  --arg buf "$(buf --version)" \
  --arg protoc "$(protoc --version)" \
  '{
    schema_version: 2,
    version: $version,
    revision: $revision,
    rust_crate: {name: $crate_name, version: $crate_version,
      asset: $crate_asset, repository_release: $crate_repository_release,
      revision: $crate_revision, sha256: $crate_sha256,
      included: $crate_included},
    tools: {go: $go, rust: $rust, buf: $buf, protoc: $protoc}
  }' >"${output}/provenance.json"

(
  cd "${output}"
  mapfile -t directory_fixtures < <(
    find metaserver-directory-v1 -type f -print | LC_ALL=C sort
  )
  checksum_files=("${archive}")
  if [[ ${crate_included} == true ]]; then
    checksum_files+=("${crate_asset}")
  fi
  checksum_files+=(atrinik-game-v1.binpb framing.json \
    metaserver-directory-v1.json metaserver-game-publisher-v1.json \
    metaserver-publisher-v1.json metaserver-directory-v1.schema.json \
    metaserver-game-publisher-v1.schema.json metaserver-directory.md \
    metaserver-publisher.md "${directory_fixtures[@]}" sbom.cdx.json \
    provenance.json THIRD_PARTY_NOTICES.md LICENSE)
  sha256sum "${checksum_files[@]}" >SHA256SUMS
)
