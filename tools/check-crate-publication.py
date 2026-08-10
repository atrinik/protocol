#!/usr/bin/env python3
"""Fail-closed contract check for the one-time crates.io bootstrap workflow."""

from __future__ import annotations

import json
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
WORKFLOW = ROOT / ".github" / "workflows" / "publish-crate.yml"
POLICY = ROOT / "policy" / "rust-crate-release.json"
FORBIDDEN = (
    "permissions: write-all",
    "--allow-dirty",
    "--token",
    "inputs.",
)
EXPECTED_HEADER = """name: Register Rust crate

on:
  workflow_dispatch:

permissions:
  contents: read
"""


def main() -> None:
    policy = json.loads(POLICY.read_text(encoding="utf-8"))
    expected_policy = {
        "schema_version": 1,
        "name": "atrinik-protocol",
        "version": "0.1.0",
        "repository_release": "1.4.0",
        "revision": "47b821a16ba955bebc79fc31e3b3bada8d74b33e",
        "asset": "atrinik-protocol-0.1.0.crate",
        "sha256": "413c4da6c1b304d4a622065efe0d36c3f591041972f1a5ee76c538926f3c0b6b",
    }
    if policy != expected_policy:
        raise SystemExit("reviewed Rust crate release policy changed")

    expected = (
        "name: Register Rust crate",
        "  workflow_dispatch:",
        "  contents: read",
        "  cancel-in-progress: false",
        "    environment: crates-io-bootstrap",
        f"      CRATE_ASSET: {policy['asset']}",
        f"      CRATE_NAME: {policy['name']}",
        f"      CRATE_SHA256: {policy['sha256']}",
        f"      CRATE_VERSION: {policy['version']}",
        f"      RELEASE_REVISION: {policy['revision']}",
        f"      RELEASE_TAG: v{policy['repository_release']}",
        "        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1",
        "          persist-credentials: false",
        f"          ref: {policy['revision']}",
        "          CARGO_REGISTRY_TOKEN: ${{ secrets.CARGO_REGISTRY_BOOTSTRAP_TOKEN }}",
        "          cargo publish --locked --no-verify --registry crates-io",
        "          unset CARGO_REGISTRY_TOKEN",
    )
    workflow = WORKFLOW.read_text(encoding="utf-8")
    if not workflow.startswith(EXPECTED_HEADER):
        raise SystemExit("bootstrap workflow trigger or permissions changed")
    for fragment in expected:
        if fragment not in workflow:
            raise SystemExit(f"missing crates.io workflow contract: {fragment}")
    for fragment in FORBIDDEN:
        if fragment in workflow:
            raise SystemExit(f"forbidden crates.io workflow contract: {fragment}")
    if workflow.count("CARGO_REGISTRY_BOOTSTRAP_TOKEN") != 1:
        raise SystemExit("bootstrap token must appear in exactly one step")
    if workflow.count("cargo publish") != 1:
        raise SystemExit("bootstrap workflow must have exactly one publish command")
    if workflow.count("--no-verify") != 1:
        raise SystemExit("only the reviewed publish command may skip duplicate build")
    if workflow.count("workflow_dispatch:") != 1:
        raise SystemExit("bootstrap workflow must be manually dispatched only")
    if any(line.strip().endswith(": write") for line in workflow.splitlines()):
        raise SystemExit("bootstrap workflow may not request write permissions")


if __name__ == "__main__":
    main()
