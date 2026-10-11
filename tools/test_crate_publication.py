"""Network-free tests for isolated builds and inert publication verification."""
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

class ProtectedVerificationTests(unittest.TestCase):
    """Exercise the protected CLI with real archive/network parsers, no builds."""

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name) / 'checkout'
        self.root.mkdir()
        self.output = self.root.parent / 'verified'
        self.github_output = self.root.parent / 'github-output'
        self.tag = 'v2.7.0'
        self.sha = 'd' * 40
        entries = {
            '.cargo_vcs_info.json': json.dumps({'git': {'sha1': REVISION}, 'path_in_vcs': 'crates/atrinik-protocol'}).encode(),
            'Cargo.toml': b'[package]\nname="atrinik-protocol"\nversion="0.2.0"\npublish=["crates-io"]\n',
            'Cargo.lock': b'version=4\n[[package]]\nname="atrinik-protocol"\nversion="0.2.0"\n',
        }
        stream = io.BytesIO()
        with tarfile.open(fileobj=stream, mode='w:gz') as archive:
            for name, data in entries.items():
                member = tarfile.TarInfo(f'{P.NAME}-{P.VERSION}/{name}')
                member.size = len(data)
                archive.addfile(member, io.BytesIO(data))
        self.data = stream.getvalue()
        self.inventory = '\n'.join(entries)
        self.artifact = {**ARTIFACT, 'sha256': P.hashlib.sha256(self.data).hexdigest()}
        self.policy_path = self.root / 'policy/rust-crate-next.json'
        self.policy_path.parent.mkdir()
        self.policy_path.write_text(json.dumps(PublicationTests().policy(self.artifact)))
        payloads = {P.ASSET: self.data, 'provenance.json': json.dumps({'revision': REVISION, 'version': self.tag[1:]}).encode()}
        payloads['SHA256SUMS'] = ''.join(P.hashlib.sha256(data).hexdigest() + '  ' + name + '\n' for name, data in payloads.items()).encode()
        base = f'https://github.com/atrinik/protocol/releases/download/{self.tag}/'
        self.released = {'id': 42, 'tag_name': self.tag, 'draft': False,
                         'prerelease': False, 'published_at': '2026-01-01', 'assets': [
                             {'name': name, 'state': 'uploaded', 'size': len(data),
                              'digest': 'sha256:' + P.hashlib.sha256(data).hexdigest(),
                              'browser_download_url': base + name}
                             for name, data in payloads.items()]}
        self.responses = {base + name: data for name, data in payloads.items()}
        self.ref_url = f'{P.API}/git/ref/tags/{self.tag}'
        self.release_url = f'{P.API}/releases/tags/{self.tag}'
        self.responses.update({
            self.ref_url: json.dumps({'object': {'type': 'commit', 'sha': REVISION}}).encode(),
            self.release_url: json.dumps(self.released).encode(),
            f'https://crates.io/api/v1/crates/{P.NAME}': json.dumps({'crate': {'id': P.NAME, 'name': P.NAME, 'repository': 'https://github.com/atrinik/protocol'}}).encode(),
            f'https://crates.io/api/v1/crates/{P.NAME}/owners': json.dumps({'users': [{'kind': 'user', 'id': 437663, 'login': 'zoeyrose', 'github_username_matches': True}]}).encode(),
            f'https://crates.io/api/v1/crates/{P.NAME}/{P.VERSION}': None,
            f'https://index.crates.io/at/ri/{P.NAME}': None,
        })
        environment = {
            'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': 'atrinik/protocol',
            'GITHUB_REPOSITORY_ID': '1327106950', 'GITHUB_REF': 'refs/heads/main',
            'GITHUB_EVENT_NAME': 'workflow_dispatch',
            'GITHUB_WORKFLOW_REF': 'atrinik/protocol/.github/workflows/publish-crate.yml@refs/heads/main',
            'GITHUB_SHA': self.sha, 'GITHUB_WORKFLOW_SHA': self.sha,
            'GITHUB_OUTPUT': str(self.github_output),
            'ACTIONS_ID_TOKEN_REQUEST_URL': 'https://synthetic.invalid',
            'ACTIONS_ID_TOKEN_REQUEST_TOKEN': 'synthetic-test-only',
        }
        self.enterContext(patch.dict(os.environ, environment, clear=True))
        self.enterContext(patch.object(P, 'ROOT', self.root))
        self.enterContext(patch('sys.argv', ['checker', 'verify-upload', '--source-tag', self.tag,
                                           '--source-revision', REVISION, '--output', str(self.output)]))
        # An allowlist at the actual process boundary fails on Cargo, rustup,
        # worktree creation, shell execution, or any other executable operation.
        self.git_outputs = {
            ('git', 'rev-parse', 'HEAD'): self.sha,
            ('git', 'status', '--porcelain'): '',
            ('git', 'rev-parse', '--verify', self.tag + '^{commit}'): REVISION,
            ('git', 'show', REVISION + ':policy/rust-crate-files.txt'): self.inventory,
        }
        self.enterContext(patch.object(P.subprocess, 'check_output', side_effect=lambda command, **kwargs: self.git_outputs[tuple(command)]))
        def run(command, **kwargs):
            self.assertEqual(command, ['git', 'merge-base', '--is-ancestor', REVISION, 'origin/main'])
        self.enterContext(patch.object(P.subprocess, 'run', side_effect=run))
        self.real_prepare = P.prepare
        self.prepare = self.enterContext(patch.object(P, 'prepare', side_effect=AssertionError('protected job must never reproduce')))
        self.archive_check = self.enterContext(patch.object(P, 'verify_crate', wraps=P.verify_crate))
        self.second_read = lambda: None
        self.ref_reads = 0
        def download(url, absent=False):
            if url == self.ref_url:
                self.ref_reads += 1
                if self.ref_reads == 2:
                    self.second_read()
            return self.responses[url]
        self.enterContext(patch.object(P, 'download', side_effect=download))

    def test_protected_cli_only_reads_inert_bytes_with_oidc_authority(self):
        P.main()
        self.assertEqual((self.output / P.ASSET).read_bytes(), self.data)
        self.archive_check.assert_called_once_with(self.data, REVISION, self.inventory.splitlines())
        self.assertIs(self.archive_check.call_args.args[0], self.data)
        self.assertEqual(self.github_output.read_text(), 'ready_for_upload=true\n')
        self.assertEqual(self.ref_reads, 2)
        self.prepare.assert_not_called()

    def test_protected_cli_fails_closed_on_fresh_evidence_drift(self):
        changes = {
            'remote source tag drift': lambda: self.responses.update({self.ref_url: json.dumps({'object': {'type': 'commit', 'sha': 'e' * 40}}).encode()}),
            'release identity changed': lambda: self.responses.update({self.release_url: json.dumps({**self.released, 'id': 43}).encode()}),
            'release API asset digest mismatch': lambda: self.responses.update({f'https://github.com/atrinik/protocol/releases/download/{self.tag}/{P.ASSET}': b'changed'}),
            'public registry owner identity changed': lambda: self.responses.update({f'https://crates.io/api/v1/crates/{P.NAME}/owners': b'{"users":[]}'}),
            'release checksums do not cover exact asset inventory': lambda: self.responses.update({self.release_url: json.dumps({**self.released, 'assets': self.released['assets'] + [{'name': 'extra', 'state': 'uploaded', 'size': 1, 'digest': 'sha256:' + DIGEST}]}).encode()}),
            'publication checkout or policy changed': lambda: self.git_outputs.update({('git', 'status', '--porcelain'): ' M tools/crate_publication.py'}),
        }
        original = dict(self.responses)
        for error, change in changes.items():
            with self.subTest(error=error):
                self.responses = dict(original)
                self.git_outputs[('git', 'status', '--porcelain')] = ''
                self.ref_reads = 0
                self.second_read = change
                with self.assertRaisesRegex(ValueError, error):
                    P.main()
                self.assertFalse(self.output.exists())
                self.assertFalse(self.github_output.exists())

    def test_protected_cli_rejects_unreviewed_archive_even_with_matching_release_metadata(self):
        self.policy_path.write_text(json.dumps(PublicationTests().policy(ARTIFACT)))
        with self.assertRaisesRegex(ValueError, 'differs from reviewed pins'):
            P.main()
        self.assertFalse(self.output.exists())

    def test_protected_cli_rejects_registry_uncertainty(self):
        self.responses[f'https://crates.io/api/v1/crates/{P.NAME}/{P.VERSION}'] = json.dumps({'version': {'crate': P.NAME, 'num': P.VERSION, 'yanked': False, 'checksum': self.artifact['sha256']}}).encode()
        with self.assertRaisesRegex(ValueError, 'registry result indeterminate'):
            P.main()
        self.assertFalse(self.github_output.exists())

    def test_protected_cli_rejects_policy_drift_before_readiness(self):
        def change_policy():
            self.policy_path.write_text(json.dumps(PublicationTests().policy(ARTIFACT)))
        self.second_read = change_policy
        with self.assertRaisesRegex(ValueError, 'publication checkout or policy changed'):
            P.main()
        self.assertFalse(self.github_output.exists())

    def test_protected_cli_rejects_wrong_workflow_revision(self):
        os.environ['GITHUB_WORKFLOW_SHA'] = 'e' * 40
        with self.assertRaisesRegex(ValueError, 'unexpected workflow revision'):
            P.main()
        self.assertEqual(self.ref_reads, 0)

    def test_public_registry_verification_still_works_in_oidc_job(self):
        digest = self.artifact['sha256']
        self.responses[f'https://crates.io/api/v1/crates/{P.NAME}/{P.VERSION}'] = json.dumps({'version': {'crate': P.NAME, 'num': P.VERSION, 'yanked': False, 'checksum': digest}}).encode()
        self.responses[f'https://index.crates.io/at/ri/{P.NAME}'] = json.dumps({'name': P.NAME, 'vers': P.VERSION, 'yanked': False, 'cksum': digest}).encode()
        with patch('sys.argv', ['checker', 'verify-registry']):
            P.main()
        self.prepare.assert_not_called()
        self.assertFalse(self.output.exists())

    def test_credential_free_reproduction_still_packages_twice_and_checks_release(self):
        self.git_outputs[('rustc', '+1.97.1', '--version')] = 'rustc 1.97.1 synthetic'
        packages = []
        def run(command, **kwargs):
            if command[:3] == ['git', 'worktree', 'add']:
                source = Path(command[4])
                (source / 'crates/atrinik-protocol').mkdir(parents=True)
                (source / 'crates/atrinik-protocol/Cargo.toml').write_text('[package]\npublish=["crates-io"]\n')
                (source / 'policy').mkdir()
                (source / 'policy/rust-crate-files.txt').write_text(self.inventory)
            elif command[:3] == ['cargo', '+1.97.1', 'package']:
                self.assertIn('--locked', command)
                self.assertFalse(os.environ.get('ACTIONS_ID_TOKEN_REQUEST_URL'))
                self.assertFalse(os.environ.get('ACTIONS_ID_TOKEN_REQUEST_TOKEN'))
                target = Path(command[-1]) / 'package' / P.ASSET
                target.parent.mkdir(parents=True)
                target.write_bytes(self.data)
                packages.append(target)
            else:
                self.assertEqual(command, ['git', 'merge-base', '--is-ancestor', REVISION, 'origin/main'])
        with patch.dict(os.environ, {}, clear=True), patch.object(P.subprocess, 'run', side_effect=run):
            self.real_prepare(self.tag, REVISION, self.output, P.load_policy(self.policy_path), True)
        self.assertEqual(len(packages), 2)
        self.assertNotEqual(packages[0], packages[1])
        self.assertEqual((self.output / P.ASSET).read_bytes(), self.data)
        self.assertEqual(self.archive_check.call_count, 2)
        self.assertEqual(self.ref_reads, 2)

    def test_packaging_rejects_oidc_before_any_source_or_build_operation(self):
        # Call the real implementation hidden by the protected-path tripwire.
        with patch.object(P, 'release', side_effect=AssertionError('source reached')):
            for variable in ('ACTIONS_ID_TOKEN_REQUEST_URL', 'ACTIONS_ID_TOKEN_REQUEST_TOKEN'):
                environment = {variable: 'synthetic-test-only'}
                with self.subTest(variable=variable), patch.dict(os.environ, environment, clear=True):
                    with self.assertRaisesRegex(ValueError, 'without OIDC request authority'):
                        self.real_prepare(self.tag, REVISION, self.output, {}, False)


if __name__ == '__main__':
    unittest.main()
