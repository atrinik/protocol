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
SOURCE_WORKFLOW = WORKFLOWS / "release.yml"
EXPECTED_SOURCE_WORKFLOW_SHA256 = "818d6584dc464997c8fb1021243159ceac2096a8d73d186462930fc7fc904d29"
EXPECTED_PUBLICATION_WORKFLOW_SHA256 = "b0491b93c39cb1b58e73afff2ac49a9aea8854a53c1de2df04a66ca639ffe58a"
EXPECTED_WORKFLOWS = {
    "dependency-review.yml", "pr-title.yml", "release.yml", "validate.yml",
    "publish-crate.yml", "crate-release-reconcile.yml",
}

EXPECTED_FUTURE_POLICY = {
    "status": "reviewed-automatic-publication",
    "authentication": "trusted-publishing",
    "repository_owner": "atrinik",
    "repository": "protocol",
    "workflow_filename": "publish-crate.yml",
    "environment": "crates-io-release",
}

# Shared hosted-runner addresses exhaust GitHub's anonymous API quota, so the
# source job's release catalog reads carry its read-only token, and only there.
SOURCE_READ_TOKEN_STEPS = (
    "Plan the actual source version without write credentials",
    "Prepare every immutable source and crate asset",
)
SOURCE_READ_TOKEN = "\n          GH_READ_TOKEN: ${{ github.token }}\n"

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


def workflow_permissions(text: str) -> list[str] | None:
    """Entries of the one top-level permissions block; YAML blank lines don't end it."""
    # splitlines() also splits on CR, NEL, U+2028 and U+2029, which YAML parsers
    # may treat as line breaks, so a comment cannot hide a further scope.
    lines = text.splitlines()
    heads = [index for index, line in enumerate(lines) if line.startswith("permissions:")]
    if len(heads) != 1 or lines[heads[0]] != "permissions:":
        return None
    entries = []
    for line in lines[heads[0] + 1:]:
        if line and not line[0].isspace() and not line.startswith("#"):
            break
        if line.strip() and not line.strip().startswith("#"):
            entries.append(line.strip())
    return entries


def check_source_read_token(text: str) -> None:
    start, end = text.find("\n  package:\n"), text.find("\n  attest:\n")
    if start < 0 or end < start:
        raise SystemExit("source package job is missing")
    job = text[start:end]
    if workflow_permissions(text) != ["contents: read"] or "permissions" in job:
        raise SystemExit("source package job must keep the read-only workflow token")
    steps = job.split("\n      - ")[1:]
    carrying = [
        step.partition("\n")[0].removeprefix("name: ")
        for step in steps
        if "\n        env:\n" in step and SOURCE_READ_TOKEN in step
    ]
    # Count the credential as well as the name, so no alias, anchor (including
    # one defined before the job), secret or whole-context dump carries it to
    # another step. Expressions are case-insensitive and allow whitespace.
    expressions = "".join(text[:end].lower().split())
    if (carrying != list(SOURCE_READ_TOKEN_STEPS) or job.count("GH_READ_TOKEN") != len(carrying)
            or expressions.count("github.token") != len(carrying) or "secrets" in job.lower()
            or "tojson(github" in expressions or "github[" in expressions):
        raise SystemExit("source catalog reads must use the read-only token in exactly the planning and preparation steps")


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
            "status": "unpublished", "publication": "reviewed-automatic-publication",
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

    check_source_read_token(SOURCE_WORKFLOW.read_text(encoding="utf-8"))
    for workflow in workflow_paths:
        text = workflow.read_text(encoding="utf-8")
        for fragment in FORBIDDEN_WORKFLOW_FRAGMENTS:
            if (workflow == SOURCE_WORKFLOW and fragment == "id-token: write"
                    and hashlib.sha256(workflow.read_bytes()).hexdigest() == EXPECTED_SOURCE_WORKFLOW_SHA256):
                continue
            if workflow != PUBLISH_WORKFLOW and fragment in text:
                raise SystemExit(
                    f"disabled registry capability in {workflow.name}: {fragment}"
                )

    readme = (ROOT / "README.md").read_text(encoding="utf-8")
    guide = (ROOT / "AGENTS.md").read_text(encoding="utf-8")
    required_readme = (
        "The one-use bootstrap workflow",
        "has been removed",
        "reviewed-automatic-publication",
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
