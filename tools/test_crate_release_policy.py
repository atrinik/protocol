#!/usr/bin/env python3
"""Regression tests for the disabled post-bootstrap registry boundary."""

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

    def test_current_policy_is_disabled_and_valid(self) -> None:
        result = self.run_check()
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_rejects_policy_drift(self) -> None:
        path = self.root / "policy" / "rust-crate-publishing.json"
        policy = json.loads(path.read_text(encoding="utf-8"))
        policy["future"]["status"] = "enabled"
        path.write_text(json.dumps(policy), encoding="utf-8")
        self.assert_rejected("reviewed Rust registry policy changed")

    def test_rejects_publish_workflow_reintroduction(self) -> None:
        path = self.root / ".github" / "workflows" / "publish-crate.yml"
        path.write_text("name: Publish\n", encoding="utf-8")
        self.assert_rejected("Rust registry publication is not activated")

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
