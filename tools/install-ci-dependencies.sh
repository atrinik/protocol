#!/usr/bin/env bash
set -euo pipefail

test -n "${RUNNER_TEMP:-}"
test -n "${GITHUB_PATH:-}"
install -d "${RUNNER_TEMP}/bin" "${RUNNER_TEMP}/protoc"

rustup toolchain install 1.97.1 --profile minimal --component clippy,rustfmt \
  --target x86_64-pc-windows-gnu
go install github.com/bufbuild/buf/cmd/buf@v1.72.0
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
cargo +1.97.1 install --locked --version 0.5.0 protoc-gen-prost

curl --fail --silent --show-error --location \
  https://github.com/protocolbuffers/protobuf/releases/download/v35.0/protoc-35.0-linux-x86_64.zip \
  --output "${RUNNER_TEMP}/protoc.zip"
printf '%s  %s\n' a45cda0989c17dd950db55f6fbe1e5814c50fda08e87aa422980ac1f89dddbbc \
  "${RUNNER_TEMP}/protoc.zip" | sha256sum --check --strict
unzip -q "${RUNNER_TEMP}/protoc.zip" -d "${RUNNER_TEMP}/protoc"

curl --fail --silent --show-error --location \
  https://github.com/anchore/syft/releases/download/v1.50.0/syft_1.50.0_linux_amd64.tar.gz \
  --output "${RUNNER_TEMP}/syft.tar.gz"
printf '%s  %s\n' bf7b29ff57f06da30918266a0e1c2885a8f99784798d1bdb1628886aa015d788 \
  "${RUNNER_TEMP}/syft.tar.gz" | sha256sum --check --strict
tar -xzf "${RUNNER_TEMP}/syft.tar.gz" -C "${RUNNER_TEMP}/bin" syft

printf '%s\n' "$(go env GOPATH)/bin" "${CARGO_HOME:-${HOME}/.cargo}/bin" \
  "${RUNNER_TEMP}/protoc/bin" "${RUNNER_TEMP}/bin" >>"${GITHUB_PATH}"
