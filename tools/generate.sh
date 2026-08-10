#!/usr/bin/env bash
set -euo pipefail

repository=$(git rev-parse --show-toplevel)
cd "${repository}"

test "$(buf --version)" = 1.72.0
test "$(protoc --version)" = "libprotoc 35.0"
test "$(protoc-gen-go --version)" = "protoc-gen-go v1.36.11"
test "$(protoc-gen-prost --version)" = 0.5.0

export BUF_CACHE_DIR=${BUF_CACHE_DIR:-/tmp/atrinik-protocol-buf-cache}

buf format --write
buf lint
buf generate
find crates/atrinik-protocol/src/generated -type f -name '*.rs' -exec sed -i \
  '1i// Copyright 2026 The Atrinik Project\n// SPDX-License-Identifier: MIT\n' \
  {} +
install -d gen/descriptor
buf build --as-file-descriptor-set \
  --output gen/descriptor/atrinik-game-v1.binpb
