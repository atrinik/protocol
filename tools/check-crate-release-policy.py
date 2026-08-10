#!/usr/bin/env python3
"""Fail closed until a reviewed trusted-publishing release is activated."""

from __future__ import annotations

import json
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
PUBLISHING_POLICY = ROOT / "policy" / "rust-crate-publishing.json"
RELEASE_POLICY = ROOT / "policy" / "rust-crate-release.json"
WORKFLOWS = ROOT / ".github" / "workflows"
PUBLISH_WORKFLOW = WORKFLOWS / "publish-crate.yml"
BOOTSTRAP_CHECK = ROOT / "tools" / "check-crate-publication.py"
EXPECTED_WORKFLOWS = {"pr-title.yml", "release.yml", "validate.yml"}

EXPECTED_FUTURE_POLICY = {
    "status": "disabled-until-reviewed-activation",
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

    if PUBLISH_WORKFLOW.exists():
        raise SystemExit("Rust registry publication is not activated")
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
            if fragment in text:
                raise SystemExit(
                    f"disabled registry capability in {workflow.name}: {fragment}"
                )

    readme = (ROOT / "README.md").read_text(encoding="utf-8")
    guide = (ROOT / "AGENTS.md").read_text(encoding="utf-8")
    required_readme = (
        "The one-use bootstrap workflow",
        "has been removed",
        "disabled-until-reviewed-activation",
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
