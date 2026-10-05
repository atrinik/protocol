"""Network-free negative tests for the credential-free publication verifier."""
import importlib.util
import io
import json
import os
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('publication', Path(__file__).with_name('crate_publication.py'))
P = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(P)
REVISION = 'a' * 40
DIGEST = 'b' * 64
ARTIFACT = {'repository_release': '2.7.0', 'revision': REVISION,
            'asset': P.ASSET, 'sha256': DIGEST}


class PublicationTests(unittest.TestCase):
    def policy(self, artifact=None):
        return {'schema_version': 1, 'name': P.NAME, 'version': P.VERSION,
                'status': 'awaiting-source-release' if artifact is None else 'ready-for-publication',
                'artifact': artifact}

    def test_pending_policy_cannot_publish(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'policy.json'
            path.write_text(json.dumps(self.policy()))
            with patch.dict(os.environ, {}, clear=True), patch('sys.argv', ['checker', 'verify-publish', '--policy', str(path)]), patch.object(P, 'prepare') as prepare:
                with self.assertRaisesRegex(ValueError, 'publication disabled'):
                    P.main()
                prepare.assert_not_called()

    def test_pending_policy_rejects_fake_pins(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'policy.json'
            value = self.policy()
            value['artifact'] = ARTIFACT
            path.write_text(json.dumps(value))
            with self.assertRaisesRegex(ValueError, 'must not invent'):
                P.load_policy(path)

    def test_artifact_requires_complete_exact_pins(self):
        P.validate_artifact(ARTIFACT)
        for key, value in [('revision', 'main'), ('sha256', 'unknown'), ('asset', '../x'), ('repository_release', 'latest')]:
            with self.subTest(key=key), self.assertRaises(ValueError):
                P.validate_artifact({**ARTIFACT, key: value})
        with self.assertRaises(ValueError):
            P.validate_artifact({})

    def test_registry_states_and_conflict(self):
        def values(api, index):
            return [None if api is None else json.dumps({'version': {'crate': P.NAME, 'num': P.VERSION, 'yanked': False, 'checksum': api}}).encode(),
                    None if index is None else json.dumps({'name': P.NAME, 'vers': P.VERSION, 'yanked': False, 'cksum': index}).encode()]
        for api, index, expected in [(None, None, 'absent'), (DIGEST, DIGEST, 'present'), (DIGEST, None, 'indeterminate')]:
            with patch.object(P, 'download', side_effect=values(api, index)):
                self.assertEqual(P.registry_state(DIGEST), expected)
        with patch.object(P, 'download', side_effect=values('c' * 64, DIGEST)), self.assertRaisesRegex(ValueError, 'conflict'):
            P.registry_state(DIGEST)

    def test_registry_package_and_owner_identity(self):
        package = {'crate': {'id': P.NAME, 'name': P.NAME,
                            'repository': 'https://github.com/atrinik/protocol'}}
        owners = {'users': [{'kind': 'user', 'id': 437663, 'login': 'zoeyrose',
                             'github_username_matches': True}]}
        with patch.object(P, 'download', side_effect=[json.dumps(package).encode(), json.dumps(owners).encode()]):
            P.verify_registry_identity()
        for bad_package, bad_owners in (
            ({'crate': {**package['crate'], 'repository': 'https://example.invalid'}}, owners),
            (package, {'users': [{**owners['users'][0], 'id': 1}]}),
            (package, {'users': []}),
        ):
            with self.subTest(package=bad_package, owners=bad_owners), patch.object(P, 'download', side_effect=[json.dumps(bad_package).encode(), json.dumps(bad_owners).encode()]):
                with self.assertRaisesRegex(ValueError, 'identity changed'):
                    P.verify_registry_identity()

    def test_publish_inputs_must_match_reviewed_pins(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'policy.json'
            path.write_text(json.dumps(self.policy(ARTIFACT)))
            for tag, revision in [('v9.0.0', REVISION), ('v2.7.0', 'c' * 40)]:
                argv = ['checker', 'verify-publish', '--policy', str(path), '--output', str(Path(temporary) / 'out'), '--source-tag', tag, '--source-revision', revision]
                with patch.dict(os.environ, {}, clear=True), patch('sys.argv', argv), patch.object(P, 'prepare') as prepare:
                    with self.assertRaisesRegex(ValueError, 'inputs differ'):
                        P.main()
                    prepare.assert_not_called()

    def test_rejects_registry_credentials_and_wrong_workflow(self):
        with patch.dict(os.environ, {'CARGO_REGISTRY_TOKEN': 'synthetic-test-only'}, clear=True), self.assertRaisesRegex(ValueError, 'without registry credentials'):
            P.execution_boundary()
        with patch.dict(os.environ, {'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': 'other/protocol'}, clear=True), self.assertRaisesRegex(ValueError, 'workflow identity'):
            P.execution_boundary()

    def test_accepts_exact_manual_workflow_identity(self):
        environment = {
            'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': 'atrinik/protocol',
            'GITHUB_REPOSITORY_ID': '1327106950', 'GITHUB_REF': 'refs/heads/main',
            'GITHUB_EVENT_NAME': 'workflow_dispatch',
            'GITHUB_WORKFLOW_REF': 'atrinik/protocol/.github/workflows/publish-crate.yml@refs/heads/main',
        }
        with patch.dict(os.environ, environment, clear=True):
            P.execution_boundary()

    def test_tag_drift_fails_before_packaging(self):
        with patch.object(P, 'git', return_value='c' * 40), patch.object(P, 'download') as download:
            with self.assertRaisesRegex(ValueError, 'source tag drift'):
                P.release('v2.7.0', REVISION)
            download.assert_not_called()
        with patch.object(P, 'git', return_value=REVISION), patch.object(P.subprocess, 'run'), patch.object(P, 'download', return_value=json.dumps({'object': {'type': 'commit', 'sha': 'c' * 40}}).encode()):
            with self.assertRaisesRegex(ValueError, 'remote source tag drift'):
                P.release('v2.7.0', REVISION)

    def test_asset_mismatch_and_redirect_are_rejected(self):
        value = {'assets': [{'name': P.ASSET, 'state': 'uploaded', 'size': 3,
                 'browser_download_url': f'https://github.com/atrinik/protocol/releases/download/v2.7.0/{P.ASSET}', 'digest': 'sha256:' + DIGEST}]}
        with patch.object(P, 'download', return_value=b'bad'), self.assertRaisesRegex(ValueError, 'digest mismatch'):
            P.release_asset(value, 'v2.7.0', P.ASSET)
        with self.assertRaisesRegex(ValueError, 'redirect'):
            P.SafeRedirect().redirect_request(None, None, 302, '', {}, 'http://attacker.invalid/')

    def test_release_manifest_covers_exact_flat_asset_inventory(self):
        manifest = (DIGEST + '  ' + P.ASSET + '\n').encode()
        def asset(name, digest):
            return {'name': name, 'digest': 'sha256:' + digest, 'state': 'uploaded', 'size': 1}
        release = {'assets': [asset('SHA256SUMS', 'c' * 64), asset(P.ASSET, DIGEST)]}
        with patch.object(P, 'release_asset', return_value=manifest):
            P.verify_release_inventory(release, 'v2.8.0')
        for changed in (b'', manifest + manifest, manifest.replace(DIGEST.encode(), b'd' * 64), manifest.replace(P.ASSET.encode(), b'nested/file'), manifest + ('a' * 64 + '  extra\n').encode()):
            with self.subTest(manifest=changed), patch.object(P, 'release_asset', return_value=changed):
                with self.assertRaises(ValueError):
                    P.verify_release_inventory(release, 'v2.8.0')

    def test_crate_identity_inventory_and_dependency_checks(self):
        entries = {
            '.cargo_vcs_info.json': json.dumps({'git': {'sha1': REVISION}, 'path_in_vcs': 'crates/atrinik-protocol'}).encode(),
            'Cargo.toml': b'[package]\nname="atrinik-protocol"\nversion="0.2.0"\npublish=["crates-io"]\n',
            'Cargo.lock': b'version=4\n[[package]]\nname="atrinik-protocol"\nversion="0.2.0"\n',
        }
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / P.ASSET
            def write(data):
                with tarfile.open(path, 'w:gz') as archive:
                    for name, content in data.items():
                        member = tarfile.TarInfo(f'{P.NAME}-{P.VERSION}/{name}')
                        member.size = len(content)
                        archive.addfile(member, io.BytesIO(content))
            write(entries)
            P.verify_crate(path, REVISION, entries)
            P.verify_crate(path.read_bytes(), REVISION, entries)
            for name, replacement, error in [
                ('.cargo_vcs_info.json', b'{"git":{"sha1":"wrong"}}', 'VCS'),
                ('Cargo.toml', entries['Cargo.toml'].replace(b'["crates-io"]', b'false'), 'publishable'),
                ('Cargo.toml', entries['Cargo.toml'] + b'[dependencies.x]\npath="../x"\n', 'dependency'),
                ('Cargo.lock', entries['Cargo.lock'] + b'[[package]]\nname="x"\nversion="1"\nsource="git+evil"\n', 'dependency'),
            ]:
                write({**entries, name: replacement})
                with self.subTest(name=name, error=error), self.assertRaisesRegex(ValueError, error):
                    P.verify_crate(path, REVISION, entries)
            write({**entries, 'unexpected': b'x'})
            with self.assertRaisesRegex(ValueError, 'inventory'):
                P.verify_crate(path, REVISION, entries)


if __name__ == '__main__':
    unittest.main()
