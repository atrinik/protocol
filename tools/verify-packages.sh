#!/usr/bin/env bash
set -euo pipefail
package_directory=$(realpath "${1:?package directory required}")
verification=$(mktemp -d /tmp/atrinik-protocol-package-proof.XXXXXX)
trap 'rm -rf -- "${verification}"' EXIT
mapfile -t archives < <(find "${package_directory}" -maxdepth 1 -name '*.crate' -type f)
if [[ ${#archives[@]} != 1 ]]; then
  echo 'Expected one protocol archive' >&2
  exit 1
fi
python3 - "${archives[0]}" "${verification}" <<'PY'
from pathlib import Path
import sys
import tomllib
from tools.crate_release import archive
path = Path(sys.argv[1])
version = path.name.removeprefix('atrinik-protocol-').removesuffix('.crate')
files = archive(path.read_bytes(), 'atrinik-protocol', version)
output = Path(sys.argv[2])
for name, data in files.items():
    target = output / name
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(data)
PY
cargo build --lib --locked --manifest-path "${verification}/Cargo.toml" --target-dir "${verification}/target"
cargo test --doc --locked --manifest-path "${verification}/Cargo.toml" --target-dir "${verification}/target"
