#!/usr/bin/env bash
set -euo pipefail

repository=$(git rev-parse --show-toplevel)
cd "${repository}"

jq -e '
  .schema_version == 1
  and (.allowed_spdx | length > 0)
  and (.forbidden_spdx | length > 0)
  and (.direct_dependencies | length == 3)
  and all(.direct_dependencies[];
    (.ecosystem == "cargo" or .ecosystem == "go")
    and ([.name, .version, .license, .source, .owner, .review_cadence,
      .eol_response, .validation] | all(. != ""))
  )
' policy/dependencies.json >/dev/null

cargo_metadata=$(mktemp /tmp/atrinik-protocol-cargo-metadata.XXXXXX)
trap 'rm -f -- "${cargo_metadata}"' EXIT
cargo metadata --locked --offline --format-version 1 >"${cargo_metadata}"

if ! jq -e --slurpfile policy policy/dependencies.json '
  . as $metadata
  | all($metadata.packages[];
    (.license // "") as $expression
    | ($expression
      | gsub("[()]"; " ")
      | split(" ")
      | map(select(. != "" and . != "AND" and . != "OR"))) as $licenses
    | ($licenses | length) > 0
      and all($licenses[];
        . as $license | ($policy[0].allowed_spdx | index($license)) != null)
  )
  and all($policy[0].direct_dependencies[] | select(.ecosystem == "cargo");
    . as $dependency
    | any($metadata.packages[];
      .name == $dependency.name
      and .version == $dependency.version
      and .license == $dependency.license)
  )
' "${cargo_metadata}" >/dev/null; then
  echo "Cargo graph violates the recorded dependency/license policy." >&2
  exit 1
fi

go_metadata=$(mktemp /tmp/atrinik-protocol-go-metadata.XXXXXX)
trap 'rm -f -- "${cargo_metadata}" "${go_metadata}"' EXIT
go list -m -json all | jq -s . >"${go_metadata}"

jq -e --slurpfile policy policy/dependencies.json '
  . as $metadata
  | all($metadata[].Path; . != "github.com/atrinik/classic")
  and all($policy[0].direct_dependencies[] | select(.ecosystem == "go");
    . as $dependency
    | any($metadata[];
      .Path == $dependency.name
      and (.Version | ltrimstr("v")) == $dependency.version)
  )
' "${go_metadata}" >/dev/null
