#!/usr/bin/env python3
"""Regression tests for the reviewed registry publication boundary."""

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
            "README.md",
            "policy/rust-crate-publishing.json",
            "policy/rust-crate-release.json",
            "tools/check-crate-release-policy.py",
            "tools/package-release.sh",
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
        path.write_text(path.read_text().replace('version = "0.2.2"', 'version = "0.1.0"'))
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

    def test_rejects_publication_workflow_guard_drift(self) -> None:
        path = self.root / ".github/workflows/publish-crate.yml"
        original = path.read_text()
        for old, new in (
            ("options: [automatic]", "options: [automatic, publish]"),
            ("crate_release.py upload", "crate_release.py confirm"),
            ("      - id: auth", "      - run: cargo package --locked\n      - id: auth"),
            ("      - id: auth", "      - run: rustup toolchain install 1.97.1\n      - id: auth"),
            ("environment: crates-io-release", "environment: other"),
            ("needs: automatic-verify", "needs: []"),
            ("persist-credentials: false", "persist-credentials: true"),
            ("ref: ${{ github.sha }}", "ref: main"),
            ("3268eb413133e2960fa4ae0e963ca76380c54cd9", "v1"),
            ("contents: read", "contents: write"),
            ("id: upload", "id: other-upload"),
            ("!cancelled() && (steps.upload.outcome == 'success' || steps.upload.outcome == 'failure')", "success()"),
            ("!cancelled() && (steps.upload.outcome == 'success' || steps.upload.outcome == 'failure')", "always()"),
            ("id: upload", "id: upload\n        continue-on-error: true"),
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

    def test_source_catalog_reads_use_only_the_read_only_job_token(self) -> None:
        path = self.root / ".github" / "workflows" / "release.yml"
        original = path.read_text(encoding="utf-8")
        token = "          GH_READ_TOKEN: ${{ github.token }}\n"
        plan = original.index("        id: plan\n")
        prepare = original.index("          SOURCE_VERSION: ${{ steps.plan.outputs.version }}\n")
        placement = "source catalog reads must use the read-only token"
        permission = "source package job must keep the read-only workflow token"

        def validate_env(entry: str) -> str:
            return original.replace("        run: tools/validate.sh\n",
                                    "        env:\n          " + entry + "\n        run: tools/validate.sh\n", 1)

        for case, changed, expected in (
            ("planning step without token",
             original[:plan] + original[plan:].replace(token, "", 1), placement),
            ("preparation step without token",
             original[:prepare] + original[prepare:].replace(token, "", 1), placement),
            ("token widened to the package job",
             original.replace("    env:\n      CARGO_HOME:",
                              "    env:\n      GH_READ_TOKEN: ${{ github.token }}\n      CARGO_HOME:", 1),
             placement),
            ("token given to the aggregate validation step",
             original.replace("        run: tools/validate.sh\n", "        env:\n" + token + "        run: tools/validate.sh\n", 1),
             placement),
            ("token renamed for the aggregate validation step",
             original.replace("        run: tools/validate.sh\n",
                              "        env:\n          GH_TOKEN: ${{ github.token }}\n        run: tools/validate.sh\n", 1),
             placement),
            ("token aliased through secrets",
             original.replace("        run: cargo fetch --locked\n",
                              "        env:\n          FETCH: ${{ secrets.GITHUB_TOKEN }}\n        run: cargo fetch --locked\n", 1),
             placement),
            ("token anchored for reuse",
             original.replace(token, token + "          LEAK: &leak ${{ github.token }}\n", 1).replace(
                 "        run: tools/validate.sh\n", "        env:\n          LEAK: *leak\n        run: tools/validate.sh\n", 1),
             placement),
            ("token anchored before the package job",
             original.replace("  group: protocol-semantic-release\n",
                              "  group: protocol-semantic-release\n  leak: &leak ${{ github.token }}\n", 1).replace(
                 "        run: tools/validate.sh\n", "        env:\n          LEAK: *leak\n        run: tools/validate.sh\n", 1),
             placement),
            ("token in another letter case",
             validate_env("LEAK: ${{ github.TOKEN }}"), placement),
            ("secret by index",
             validate_env("LEAK: ${{ secrets['GITHUB_TOKEN'] }}"), placement),
            ("whole github context",
             validate_env("LEAK: ${{ toJSON(github) }}"), placement),
            ("github context by index",
             validate_env("LEAK: ${{ github [ 'token' ] }}"), placement),
            ("package job write permission",
             original.replace("    timeout-minutes: 40\n", "    timeout-minutes: 40\n    permissions:\n      contents: write\n", 1),
             permission),
            ("workflow write permission",
             original.replace("permissions:\n  contents: read\n", "permissions:\n  contents: write\n", 1), permission),
            ("workflow scopes after a blank line",
             original.replace("permissions:\n  contents: read\n\n",
                              "permissions:\n  contents: read\n\n  id-token: write\n  actions: write\n\n", 1),
             permission),
            ("workflow read-all shorthand",
             original.replace("permissions:\n  contents: read\n", "permissions: read-all\n", 1), permission),
            ("workflow scope hidden by a comment line separator",
             original.replace("permissions:\n  contents: read\n",
                              "permissions:\n  contents: read\n  # note\u2028  actions: write\n", 1), permission),
            ("package job permissions key with a space",
             original.replace("    timeout-minutes: 40\n", "    timeout-minutes: 40\n    permissions : write-all\n", 1),
             permission),
            ("package job quoted permissions key",
             original.replace("    timeout-minutes: 40\n", '    timeout-minutes: 40\n    "permissions": write-all\n', 1),
             permission),
        ):
            with self.subTest(case=case):
                self.assertNotEqual(changed, original)
                path.write_text(changed, encoding="utf-8")
                self.assert_rejected(expected)

    def test_release_script_passes_read_token_only_to_crate_preparation(self) -> None:
        path = self.root / "tools" / "package-release.sh"
        original = path.read_text(encoding="utf-8")
        scope = "declare +x catalog_read_token=${GH_READ_TOKEN-}\nunset GH_READ_TOKEN\n"
        use = "GH_READ_TOKEN=${catalog_read_token} python3 tools/crate_prepare.py prepare "
        cd = 'cd "${repository}"\n'
        for case, changed in (
            ("token left exported", original.replace("unset GH_READ_TOKEN\n", "", 1)),
            ("holder exported", original.replace("declare +x catalog_read_token", "export catalog_read_token", 1)),
            ("scope after the first command",
             original.replace(scope, "", 1).replace(cd, cd + scope, 1)),
            ("token withheld from crate preparation",
             original.replace(use, "python3 tools/crate_prepare.py prepare ", 1)),
            ("token passed to cargo package",
             original.replace("  cargo package --locked --offline --allow-dirty",
                              "  GH_READ_TOKEN=${catalog_read_token} cargo package --locked --offline --allow-dirty", 1)),
            ("token passed to the contract bundle",
             original.replace("python3 tools/contract_release_bundle.py",
                              "GH_READ_TOKEN=${catalog_read_token} python3 tools/contract_release_bundle.py", 1)),
            ("token re-exported", original.replace(use, "export GH_READ_TOKEN=${catalog_read_token}\n" + use, 1)),
            ("holder exported on a later line",
             original.replace("unset GH_READ_TOKEN\n", "unset GH_READ_TOKEN\nexport catalog_read_token\n", 1)),
            ("token passed to cargo package through indirect expansion",
             original.replace("  cargo package --locked --offline --allow-dirty",
                              "  holder=catalog_read; holder+=_token\n"
                              "  GH_READ_TOKEN=${!holder} cargo package --locked --offline --allow-dirty", 1)),
            ("token moved from crate preparation to cargo package",
             original.replace(use, "python3 tools/crate_prepare.py prepare ", 1).replace(
                 "  cargo package --locked --offline --allow-dirty",
                 "  GH_READ_TOKEN=${catalog_read_token} cargo package --locked --offline --allow-dirty", 1)),
        ):
            with self.subTest(case=case):
                self.assertNotEqual(changed, original)
                path.write_text(changed, encoding="utf-8")
                self.assert_rejected("package-release.sh must pass the read token only to crate preparation")

    def test_rejects_registry_capabilities_in_dependency_review(self) -> None:
        path = self.root / ".github" / "workflows" / "dependency-review.yml"
        original = path.read_text(encoding="utf-8")
        for fragment in ("id-token: write", "CARGO_REGISTRY_TOKEN", "cargo publish"):
            with self.subTest(fragment=fragment):
                path.write_text(original + f"\n# {fragment}\n", encoding="utf-8")
                self.assert_rejected("disabled registry capability")

    def test_rejects_bootstrap_checker_reintroduction(self) -> None:
        path = self.root / "tools" / "check-crate-publication.py"
        path.write_text("# retired\n", encoding="utf-8")
        self.assert_rejected("one-time bootstrap checker must remain removed")


if __name__ == "__main__":
    unittest.main()
