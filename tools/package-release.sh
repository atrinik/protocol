#!/usr/bin/env bash
set -euo pipefail

repository=$(git rev-parse --show-toplevel)
cd "${repository}"

output=${1:-dist}
if [[ -e ${output} ]]; then
  echo "Release output already exists: ${output}" >&2
  exit 1
fi
install -d "${output}"

version=$(git describe --tags --always --dirty)
archive="atrinik-protocol-${version}.tar.gz"
git archive --format=tar --prefix="atrinik-protocol-${version}/" HEAD \
  | gzip -n >"${output}/${archive}"
cp gen/descriptor/atrinik-game-v1.binpb "${output}/"
cp fixtures/framing.json THIRD_PARTY_NOTICES.md LICENSE "${output}/"

SYFT_CHECK_FOR_APP_UPDATE=false syft dir:. \
  --source-name atrinik-protocol --source-version "${version}" \
  --output "cyclonedx-json=${output}/sbom.cdx.json"

jq -n \
  --arg version "${version}" \
  --arg revision "$(git rev-parse HEAD)" \
  --arg go "$(go version)" \
  --arg rust "$(rustc --version)" \
  --arg buf "$(buf --version)" \
  --arg protoc "$(protoc --version)" \
  '{
    schema_version: 1,
    version: $version,
    revision: $revision,
    tools: {go: $go, rust: $rust, buf: $buf, protoc: $protoc}
  }' >"${output}/provenance.json"

(
  cd "${output}"
  sha256sum "${archive}" atrinik-game-v1.binpb framing.json \
    sbom.cdx.json provenance.json THIRD_PARTY_NOTICES.md LICENSE \
    >SHA256SUMS
)
