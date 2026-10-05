#!/usr/bin/env python3
"""Regression tests for the reviewed manual registry boundary."""

from __future__ import annotations

import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


SOURCE_ROOT = Path(__file__).resolve().parent.parent


class CrateReleasePolicyTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory(
            prefix="atrinik-protocol-crate-policy-"
        )
        self.root = Path(self.temporary.name)
        for relative in (
            "AGENTS.md",
            "Cargo.toml",
            "crates/atrinik-protocol/Cargo.toml",
            "policy/rust-crate-candidate.json",
            "policy/rust-crate-next.json",
            "README.md",
            "policy/rust-crate-publishing.json",
            "policy/rust-crate-release.json",
            "tools/check-crate-release-policy.py",
        ):
            source = SOURCE_ROOT / relative
            target = self.root / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, target)
        shutil.copytree(
            SOURCE_ROOT / ".github" / "workflows",
            self.root / ".github" / "workflows",
        )

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def run_check(self) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, "tools/check-crate-release-policy.py"],
            cwd=self.root,
            check=False,
            capture_output=True,
            text=True,
        )

    def assert_rejected(self, expected: str) -> None:
        result = self.run_check()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(expected, result.stderr)

    def test_current_policy_is_reviewed_and_valid(self) -> None:
        result = self.run_check()
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_rejects_manifest_publication(self) -> None:
        path = self.root / "crates/atrinik-protocol/Cargo.toml"
        original = path.read_text()
        for setting in ('publish = true', 'publish = false', ''):
            with self.subTest(setting=setting):
                path.write_text(original.replace('publish = ["crates-io"]', setting))
                self.assert_rejected("manifest must restrict publish to crates-io")

    def test_rejects_candidate_publication(self) -> None:
        path = self.root / "policy/rust-crate-candidate.json"
        candidate = json.loads(path.read_text())
        candidate["publication"] = "enabled"
        path.write_text(json.dumps(candidate))
        self.assert_rejected("candidate policy changed")

    def test_rejects_missing_candidate(self) -> None:
        (self.root / "policy/rust-crate-candidate.json").unlink()
        self.assert_rejected("requires explicit candidate policy")

    def test_rejects_reusing_published_version(self) -> None:
        path = self.root / "Cargo.toml"
        path.write_text(path.read_text().replace('version = "0.2.0"', 'version = "0.1.0"'))
        self.assert_rejected("cannot be relabeled")

    def test_rejects_policy_drift(self) -> None:
        path = self.root / "policy" / "rust-crate-publishing.json"
        policy = json.loads(path.read_text(encoding="utf-8"))
        policy["future"]["status"] = "enabled"
        path.write_text(json.dumps(policy), encoding="utf-8")
        self.assert_rejected("reviewed Rust registry policy changed")

    def test_rejects_publish_capability_before_artifact_review(self) -> None:
        path = self.root / ".github/workflows/publish-crate.yml"
        with path.open("a") as stream:
            stream.write("\n# id-token: write\n")
        self.assert_rejected("reviewed publication workflow changed")

    def test_rejects_unreviewed_artifact_pins(self) -> None:
        path = self.root / "policy/rust-crate-next.json"
        value = json.loads(path.read_text())
        value["artifact"] = {"revision": "a" * 40}
        path.write_text(json.dumps(value))
        self.assert_rejected("reviewed next-crate artifact pins changed")

    def test_rejects_publication_workflow_guard_drift(self) -> None:
        path = self.root / ".github/workflows/publish-crate.yml"
        original = path.read_text()
        for old, new in (
            ("default: prepare", "default: publish"),
            ("environment: crates-io-release", "environment: other"),
            ("needs.verify.outputs.ready == 'true'", "always()"),
            ("persist-credentials: false", "persist-credentials: true"),
            ("ref: ${{ github.sha }}", "ref: main"),
            ("c6f97d42243bad5fab37ca0427f495c86d5b1a18", "v1"),
            ("contents: read", "contents: write"),
        ):
            with self.subTest(old=old):
                self.assertIn(old, original)
                path.write_text(original.replace(old, new))
                self.assert_rejected("reviewed publication workflow changed")

    def test_rejects_any_unreviewed_workflow(self) -> None:
        path = self.root / ".github" / "workflows" / "other.yml"
        path.write_text("name: Other\n", encoding="utf-8")
        self.assert_rejected("reviewed workflow inventory changed")

    def test_rejects_oidc_permission_in_existing_workflow(self) -> None:
        path = self.root / ".github" / "workflows" / "release.yml"
        with path.open("a", encoding="utf-8") as stream:
            stream.write("\n# id-token: write\n")
        self.assert_rejected("disabled registry capability")

    def test_rejects_bootstrap_checker_reintroduction(self) -> None:
        path = self.root / "tools" / "check-crate-publication.py"
        path.write_text("# retired\n", encoding="utf-8")
        self.assert_rejected("one-time bootstrap checker must remain removed")


if __name__ == "__main__":
    unittest.main()
