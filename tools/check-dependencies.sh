#!/usr/bin/env bash
set -euo pipefail

repository=$(git rev-parse --show-toplevel)
cd "${repository}"

jq -e '
  . as $policy
  | .schema_version == 2
  and (.allowed_spdx | length > 0)
  and (.forbidden_spdx | length > 0)
  and (.allowed_cargo_sources | length > 0)
  and (.forbidden_go_modules | length > 0)
  and all(.allowed_spdx[]; . as $license
    | ($policy.forbidden_spdx | index($license)) == null)
  and ((.direct_dependencies | map([.ecosystem, .name] | join(":"))
    | unique | length) == (.direct_dependencies | length))
  and all(.direct_dependencies[];
    (.ecosystem == "cargo" or .ecosystem == "go")
    and ([.name, .source, .owner, .review_cadence,
      .eol_response, .validation] | all(. != ""))
    and (has("version") | not)
    and (has("license") | not)
  )
' policy/dependencies.json >/dev/null

cargo_metadata=$(mktemp /tmp/atrinik-protocol-cargo-metadata.XXXXXX)
trap 'rm -f -- "${cargo_metadata}"' EXIT
cargo metadata --locked --offline --format-version 1 >"${cargo_metadata}"

if ! jq -e --slurpfile policy policy/dependencies.json '
  def valid_license_expression($expression; $allowed; $forbidden):
    ($expression
      | gsub("\\("; " ( ")
      | gsub("\\)"; " ) ")
      | split(" ")
      | map(select(. != ""))) as $tokens
    | ($tokens | length) > 0
    and (reduce $tokens[] as $token (
      {valid: true, expect_operand: true, depth: 0};
      if (.valid | not) then
        .
      elif .expect_operand then
        if $token == "(" then
          .depth += 1
        elif (($allowed | index($token)) != null
          and ($forbidden | index($token)) == null) then
          .expect_operand = false
        else
          .valid = false
        end
      elif $token == ")" and .depth > 0 then
        .depth -= 1
      elif $token == "AND" or $token == "OR" then
        .expect_operand = true
      else
        .valid = false
      end
    ) | .valid and (.expect_operand | not) and .depth == 0);

  . as $metadata
  | ($metadata.workspace_members | unique) as $workspace_members
  | ([
      $metadata.packages[]
      | select(.id as $id | $workspace_members | index($id))
      | .dependencies[]
      | select(.kind == null and .source != null)
      | .name
    ] | unique | sort) as $manifest_dependencies
  | ([$policy[0].direct_dependencies[]
      | select(.ecosystem == "cargo") | .name] | unique | sort)
      as $policy_dependencies
  | ($manifest_dependencies == $policy_dependencies)
  and all($metadata.packages[];
    . as $package
    | if ($package.id as $id | $workspace_members | index($id)) then
        $package.source == null
      else
        ($policy[0].allowed_cargo_sources | index($package.source)) != null
      end)
  and all($metadata.packages[];
    (.license // "") as $expression
    | valid_license_expression($expression;
        $policy[0].allowed_spdx; $policy[0].forbidden_spdx))
' "${cargo_metadata}" >/dev/null; then
  echo "Cargo graph violates the dependency source, inventory, or license policy." >&2
  exit 1
fi

go_metadata=$(mktemp /tmp/atrinik-protocol-go-metadata.XXXXXX)
go_manifest=$(mktemp /tmp/atrinik-protocol-go-manifest.XXXXXX)
trap 'rm -f -- "${cargo_metadata}" "${go_metadata}" "${go_manifest}"' EXIT
go mod verify
go list -m -json all | jq -s . >"${go_metadata}"
go mod edit -json >"${go_manifest}"

jq -e --slurpfile policy policy/dependencies.json \
  --slurpfile manifest "${go_manifest}" '
  . as $metadata
  | ([$manifest[0].Require[] | select(.Indirect != true) | .Path]
      | unique | sort) as $manifest_dependencies
  | ([$policy[0].direct_dependencies[]
      | select(.ecosystem == "go") | .name] | unique | sort)
      as $policy_dependencies
  | ($manifest_dependencies == $policy_dependencies)
  and all($manifest_dependencies[];
    . as $dependency
    | any($metadata[]; .Path == $dependency))
  and all($metadata[];
    . as $module
    | if .Main == true then
      true
    else
      (.Replace == null)
      and ((.Version // "") | test("^v[^[:space:]]+$"))
      and ((.Sum == null) or (.Sum | test("^h1:[A-Za-z0-9+/=]+$")))
      and ((.GoModSum == null)
        or (.GoModSum | test("^h1:[A-Za-z0-9+/=]+$")))
      and all($policy[0].forbidden_go_modules[];
        . as $forbidden
        | $module.Path != $forbidden
        and ($module.Path | startswith($forbidden + "/") | not))
    end)
' "${go_metadata}" >/dev/null
