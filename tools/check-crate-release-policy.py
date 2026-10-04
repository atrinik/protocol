#!/usr/bin/env python3
"""Fail closed until a reviewed trusted-publishing release is activated."""

from __future__ import annotations

import json
import hashlib
import re
import tomllib
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
PUBLISHING_POLICY = ROOT / "policy" / "rust-crate-publishing.json"
RELEASE_POLICY = ROOT / "policy" / "rust-crate-release.json"
WORKFLOWS = ROOT / ".github" / "workflows"
PUBLISH_WORKFLOW = WORKFLOWS / "publish-crate.yml"
BOOTSTRAP_CHECK = ROOT / "tools" / "check-crate-publication.py"
EXPECTED_PUBLICATION_WORKFLOW_SHA256 = "94bb1b7795cafdc35dbe0e9d1741d73a946fd32a53475e85205f33ea53b316b9"
EXPECTED_WORKFLOWS = {"pr-title.yml", "release.yml", "validate.yml", "publish-crate.yml"}

EXPECTED_FUTURE_POLICY = {
    "status": "reviewed-manual-publication",
    "authentication": "trusted-publishing",
    "repository_owner": "atrinik",
    "repository": "protocol",
    "workflow_filename": "publish-crate.yml",
    "environment": "crates-io-release",
}

FORBIDDEN_WORKFLOW_FRAGMENTS = (
    "CARGO_REGISTRY_BOOTSTRAP_TOKEN",
    "CARGO_REGISTRY_TOKEN",
    "cargo publish",
    "crates-io-auth-action",
    "crates-io-bootstrap",
    "id-token: write",
)


def load_json(path: Path) -> object:
    return json.loads(path.read_text(encoding="utf-8"))


def main() -> None:
    publishing = load_json(PUBLISHING_POLICY)
    release = load_json(RELEASE_POLICY)
    workspace = tomllib.loads((ROOT / "Cargo.toml").read_text())
    workspace_version = workspace["workspace"]["package"]["version"]
    candidate_path = ROOT / "policy" / "rust-crate-candidate.json"
    if workspace_version != release["version"]:
        if not candidate_path.is_file():
            raise SystemExit("unpublished crate requires explicit candidate policy")
        manifest = tomllib.loads((ROOT / "crates/atrinik-protocol/Cargo.toml").read_text())
        if manifest["package"].get("publish") != ["crates-io"]:
            raise SystemExit("prepared crate manifest must restrict publish to crates-io")
        candidate = load_json(candidate_path)
        expected_candidate = {
            "schema_version": 1, "name": release["name"],
            "version": workspace_version, "base_published_version": release["version"],
            "status": "unpublished", "publication": "reviewed-manual-publication",
        }
        if candidate != expected_candidate or not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", workspace_version):
            raise SystemExit("unpublished crate candidate policy changed")
        if tuple(map(int, workspace_version.split("."))) <= tuple(map(int, release["version"].split("."))):
            raise SystemExit("unpublished crate must advance the immutable published version")
    elif candidate_path.exists():
        raise SystemExit("published crate cannot be relabeled as an unpublished candidate")

    expected_published = {
        key: release[key]
        for key in (
            "version",
            "repository_release",
            "revision",
            "asset",
            "sha256",
        )
    }
    expected = {
        "schema_version": 1,
        "registry": "crates-io",
        "crate": release["name"],
        "published": expected_published,
        "future": EXPECTED_FUTURE_POLICY,
    }
    if publishing != expected:
        raise SystemExit("reviewed Rust registry policy changed")

    next_policy = load_json(ROOT / "policy/rust-crate-next.json")
    expected_next = {"schema_version": 1, "name": release["name"],
                     "version": workspace_version, "status": "ready-for-publication",
                     "artifact": {
                         "repository_release": "2.8.0",
                         "revision": "a47537790f5ea8a08adf7e6dffee4fa794cf3665",
                         "asset": "atrinik-protocol-0.2.0.crate",
                         "sha256": "cda65c3c322993cfab378454fb9b1182df8a000216f4abd1170e53cdfdc3b3bb",
                     }}
    if next_policy != expected_next:
        raise SystemExit("reviewed next-crate artifact pins changed")
    if not PUBLISH_WORKFLOW.is_file():
        raise SystemExit("reviewed publication workflow is required")
    # Fence the complete reviewed job/step/permission graph, not substrings that
    # could overlook a YAML placement or expression change. A workflow edit
    # requires changing this anchor and its independent policy review together.
    if hashlib.sha256(PUBLISH_WORKFLOW.read_bytes()).hexdigest() != EXPECTED_PUBLICATION_WORKFLOW_SHA256:
        raise SystemExit("reviewed publication workflow changed")
    if BOOTSTRAP_CHECK.exists():
        raise SystemExit("one-time bootstrap checker must remain removed")

    workflow_paths = {
        path
        for pattern in ("*.yml", "*.yaml")
        for path in WORKFLOWS.glob(pattern)
    }
    if {path.name for path in workflow_paths} != EXPECTED_WORKFLOWS:
        raise SystemExit("reviewed workflow inventory changed")

    for workflow in workflow_paths:
        text = workflow.read_text(encoding="utf-8")
        for fragment in FORBIDDEN_WORKFLOW_FRAGMENTS:
            if workflow != PUBLISH_WORKFLOW and fragment in text:
                raise SystemExit(
                    f"disabled registry capability in {workflow.name}: {fragment}"
                )

    readme = (ROOT / "README.md").read_text(encoding="utf-8")
    guide = (ROOT / "AGENTS.md").read_text(encoding="utf-8")
    required_readme = (
        "The one-use bootstrap workflow",
        "has been removed",
        "reviewed-manual-publication",
        "crates-io-release",
        "Trusted Publishing",
    )
    required_guide = (
        "Trusted Publishing",
        "crates-io-release",
        "short-lived token",
    )
    for fragment in required_readme:
        if fragment not in readme:
            raise SystemExit(f"missing Rust registry README contract: {fragment}")
    for fragment in required_guide:
        if fragment not in guide:
            raise SystemExit(f"missing Rust registry agent contract: {fragment}")


if __name__ == "__main__":
    main()
