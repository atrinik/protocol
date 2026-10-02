#!/usr/bin/env bash
set -euo pipefail

repository=$(git rev-parse --show-toplevel)
cd "${repository}"

fixture_directory=$(mktemp -d /tmp/atrinik-protocol-dependency-test.XXXXXX)
cleanup() {
  rm -rf -- "${fixture_directory}"
}
trap cleanup EXIT

mkdir "${fixture_directory}/bin"

cat >"${fixture_directory}/bin/cargo" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
test "$1" = metadata
cat "${MOCK_CARGO_METADATA}"
EOF

cat >"${fixture_directory}/bin/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1 $2" in
  "list -m")
    cat "${MOCK_GO_METADATA}"
    ;;
  "mod edit")
    cat "${MOCK_GO_MANIFEST}"
    ;;
  *)
    exit 64
    ;;
esac
EOF
chmod +x "${fixture_directory}/bin/cargo" "${fixture_directory}/bin/go"

export MOCK_CARGO_METADATA="${fixture_directory}/cargo.json"
export MOCK_GO_METADATA="${fixture_directory}/go.jsons"
export MOCK_GO_MANIFEST="${fixture_directory}/go-mod.json"
export PATH="${fixture_directory}/bin:${PATH}"

write_valid_fixtures() {
  cat >"${MOCK_CARGO_METADATA}" <<'EOF'
{
  "workspace_members": ["atrinik-protocol 0.1.0 (path+file:///src)"],
  "packages": [
    {
      "id": "atrinik-protocol 0.1.0 (path+file:///src)",
      "name": "atrinik-protocol",
      "version": "0.1.0",
      "license": "MIT",
      "source": null,
      "dependencies": [
        {"name":"bytes","kind":null,"source":"registry+https://github.com/rust-lang/crates.io-index"},
        {"name":"idna","kind":null,"source":"registry+https://github.com/rust-lang/crates.io-index"},
        {"name":"prost","kind":null,"source":"registry+https://github.com/rust-lang/crates.io-index"}
      ]
    },
    {"id":"bytes 1.13.0","name":"bytes","version":"1.13.0","license":"MIT","source":"registry+https://github.com/rust-lang/crates.io-index","dependencies":[]},
    {"id":"idna 1.2.0","name":"idna","version":"1.2.0","license":"MIT OR Apache-2.0","source":"registry+https://github.com/rust-lang/crates.io-index","dependencies":[]},
    {"id":"prost 0.14.6","name":"prost","version":"0.14.6","license":"Apache-2.0","source":"registry+https://github.com/rust-lang/crates.io-index","dependencies":[]}
  ]
}
EOF
  cat >"${MOCK_GO_METADATA}" <<'EOF'
{"Path":"github.com/atrinik/protocol","Main":true}
{"Path":"github.com/google/go-cmp","Version":"v0.7.0","Indirect":true,"GoMod":"/cache/github.com/google/go-cmp@v0.7.0/go.mod","GoModSum":"h1:cXJzdA=="}
{"Path":"golang.org/x/net","Version":"v0.58.0","Sum":"h1:YWJjZA=="}
{"Path":"golang.org/x/text","Version":"v0.41.0","Sum":"h1:ZWZnaA=="}
{"Path":"google.golang.org/protobuf","Version":"v1.37.0","Sum":"h1:aWprbA=="}
EOF
  cat >"${MOCK_GO_MANIFEST}" <<'EOF'
{
  "Require": [
    {"Path":"golang.org/x/net","Version":"v0.58.0"},
    {"Path":"google.golang.org/protobuf","Version":"v1.37.0"},
    {"Path":"golang.org/x/text","Version":"v0.41.0","Indirect":true}
  ]
}
EOF
}

expect_failure() {
  if tools/check-dependencies.sh >/dev/null 2>&1; then
    echo "Dependency check unexpectedly accepted ${1}." >&2
    exit 1
  fi
}

write_valid_fixtures
tools/check-dependencies.sh

jq '(.packages[] | select(.name == "bytes") | .license) = "GPL-3.0-only"' \
  "${MOCK_CARGO_METADATA}" >"${MOCK_CARGO_METADATA}.new"
mv "${MOCK_CARGO_METADATA}.new" "${MOCK_CARGO_METADATA}"
expect_failure "a forbidden Cargo license"

write_valid_fixtures
jq '(.packages[] | select(.name == "bytes") | .source) =
  "git+https://example.invalid/bytes"' \
  "${MOCK_CARGO_METADATA}" >"${MOCK_CARGO_METADATA}.new"
mv "${MOCK_CARGO_METADATA}.new" "${MOCK_CARGO_METADATA}"
expect_failure "a Cargo Git source"

write_valid_fixtures
jq 'if .Path == "golang.org/x/net" then .Replace = {
  "Path":"example.invalid/net", "Version":"v0.58.0", "Sum":"h1:YWJjZA=="
} else . end' "${MOCK_GO_METADATA}" >"${MOCK_GO_METADATA}.new"
mv "${MOCK_GO_METADATA}.new" "${MOCK_GO_METADATA}"
expect_failure "a replaced Go module"

write_valid_fixtures
printf '%s\n' \
  '{"Path":"github.com/atrinik/classic","Version":"v2.0.0","Sum":"h1:bW5vcA=="}' \
  >>"${MOCK_GO_METADATA}"
expect_failure "a forbidden Go module"

write_valid_fixtures
jq 'if .Path == "github.com/google/go-cmp"
  then del(.GoModSum) else . end' \
  "${MOCK_GO_METADATA}" >"${MOCK_GO_METADATA}.new"
mv "${MOCK_GO_METADATA}.new" "${MOCK_GO_METADATA}"
expect_failure "a Go module without an authenticated module or archive sum"

write_valid_fixtures
jq '(.Require[] | select(.Path == "golang.org/x/text") | .Indirect) = false' \
  "${MOCK_GO_MANIFEST}" >"${MOCK_GO_MANIFEST}.new"
mv "${MOCK_GO_MANIFEST}.new" "${MOCK_GO_MANIFEST}"
expect_failure "an unreviewed direct dependency"
