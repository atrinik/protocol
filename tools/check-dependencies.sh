#!/usr/bin/env bash
set -euo pipefail

repository=$(git rev-parse --show-toplevel)
cd "${repository}"

jq -e '.schema_version == 1 and (.direct_dependencies | length == 3)' \
  policy/dependencies.json >/dev/null

cargo_metadata=$(mktemp /tmp/atrinik-protocol-cargo-metadata.XXXXXX)
trap 'rm -f -- "${cargo_metadata}"' EXIT
cargo metadata --locked --offline --format-version 1 >"${cargo_metadata}"

if jq -e '
  [.packages[] | select(
    (.license // "") | test("(^|[^A])GPL-[0-9]|AGPL-[0-9]")
  )] | length > 0
' "${cargo_metadata}" >/dev/null; then
  echo "Cargo graph contains a forbidden reciprocal license." >&2
  exit 1
fi

go list -m -json all | jq -s -e '
  all(.[].Path; . != "github.com/atrinik/classic")
' >/dev/null
