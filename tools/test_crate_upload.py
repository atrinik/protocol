"""Network-free tests for the exact-byte uploader and its fail-closed boundary."""
from contextlib import ExitStack
import hashlib
import io
import json
import os
from pathlib import Path
import struct
import tarfile
import tempfile
import unittest
from unittest.mock import Mock, patch
import urllib.error

from tools import crate_upload as U

P = U.P
REVISION = 'a' * 40
WORKFLOW_REVISION = 'b' * 40
TOKEN = 'synthetic-upload-token-for-tests'
ENVIRONMENT = {
    'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': 'atrinik/protocol',
    'GITHUB_REPOSITORY_ID': '1327106950', 'GITHUB_REF': 'refs/heads/main',
    'GITHUB_EVENT_NAME': 'workflow_dispatch',
    'GITHUB_WORKFLOW_REF': 'atrinik/protocol/.github/workflows/publish-crate.yml@refs/heads/main',
    'GITHUB_SHA': WORKFLOW_REVISION, 'GITHUB_WORKFLOW_SHA': WORKFLOW_REVISION,
    'CRATE_PUBLICATION_OPERATION': 'publish', 'CARGO_REGISTRY_TOKEN': TOKEN,
}
MANIFEST = b'''[package]
name = "atrinik-protocol"
version = "0.2.0"
edition = "2024"
rust-version = "1.97.1"
build = false
publish = ["crates-io"]
autolib = false
autobins = false
autoexamples = false
autotests = false
autobenches = false
description = "Test fixture"
readme = "README.md"
license = "MIT"
repository = "https://github.com/atrinik/protocol"
[lib]
name = "atrinik_protocol"
path = "src/lib.rs"
[dependencies.bytes]
version = "1.12.1"
[dependencies.idna]
version = "1.1.0"
[dependencies.prost]
version = "0.14.4"
'''


def fixture(manifest=MANIFEST, vcs=None):
    entries = {
        'Cargo.toml': manifest,
        'Cargo.lock': b'version=4\n[[package]]\nname="atrinik-protocol"\nversion="0.2.0"\n',
        '.cargo_vcs_info.json': json.dumps(vcs or {'git': {'sha1': REVISION}, 'path_in_vcs': 'crates/atrinik-protocol'}).encode(),
        'README.md': b'Independent test fixture README.\n',
        'src/lib.rs': b'// Independent test fixture.\n',
    }
    stream = io.BytesIO()
    with tarfile.open(fileobj=stream, mode='w:gz') as archive:
        for name, content in entries.items():
            member = tarfile.TarInfo(f'{P.NAME}-{P.VERSION}/{name}')
            member.size = len(content)
            archive.addfile(member, io.BytesIO(content))
    return stream.getvalue(), list(entries)


class Response:
    def __init__(self, body=b'{}', status=200, url=U.ENDPOINT):
        self.body, self.status, self.url = body, status, url
        self.read_bounds = []

    def __enter__(self):
        return self

    def __exit__(self, *args):
        pass

    def geturl(self):
        return self.url

    def read(self, bound):
        self.read_bounds.append(bound)
        return self.body[:bound]


class UploadTests(unittest.TestCase):
    def setUp(self):
        self.stack = ExitStack()
        self.addCleanup(self.stack.close)
        self.directory = Path(self.stack.enter_context(tempfile.TemporaryDirectory()))
        self.stack.enter_context(patch.dict(os.environ, ENVIRONMENT, clear=True))
        self.stack.enter_context(patch.object(P, 'ROOT', self.directory))
        self.sleep = self.stack.enter_context(patch.object(U.time, 'sleep'))
        self.opener = Mock()
        self.response = Response()
        self.opener.open.return_value = self.response
        self.build_opener = self.stack.enter_context(patch.object(U.urllib.request, 'build_opener', return_value=self.opener))
        self.release = self.stack.enter_context(patch.object(P, 'release', return_value={'id': 123}))
        self.release_asset = self.stack.enter_context(patch.object(P, 'release_asset'))
        self.release_inventory = self.stack.enter_context(patch.object(P, 'verify_release_inventory'))
        self.identity = self.stack.enter_context(patch.object(P, 'verify_registry_identity'))
        self.registry = self.stack.enter_context(patch.object(P, 'registry_state', return_value='absent'))
        self.git = self.stack.enter_context(patch.object(P, 'git', side_effect=self.git_result))
        self.policy_path = self.directory / 'policy/rust-crate-next.json'
        self.policy_path.parent.mkdir()
        self.crate_path = self.directory / P.ASSET
        self.write_fixture()

    def git_result(self, *args):
        self.assertNotIn('CARGO_REGISTRY_TOKEN', os.environ)
        if args == ('rev-parse', 'HEAD'):
            return WORKFLOW_REVISION
        if args == ('status', '--porcelain'):
            return ''
        self.assertEqual(args, ('show', REVISION + ':policy/rust-crate-files.txt'))
        return '\n'.join(self.inventory)

    def write_fixture(self, manifest=MANIFEST, vcs=None):
        self.data, self.inventory = fixture(manifest, vcs)
        self.crate_path.write_bytes(self.data)
        self.artifact = {'repository_release': '2.8.0', 'revision': REVISION,
                         'asset': P.ASSET, 'sha256': hashlib.sha256(self.data).hexdigest()}
        self.policy = {'schema_version': 1, 'name': P.NAME, 'version': P.VERSION,
                       'status': 'ready-for-publication', 'artifact': self.artifact}
        self.policy_path.write_text(json.dumps(self.policy))
        self.release_asset.return_value = self.data

    def upload(self):
        return U.upload(self.crate_path, self.policy_path)

    def test_exact_original_archive_framing_and_metadata(self):
        self.assertEqual(self.upload(), 'success')
        self.opener.open.assert_called_once()
        request = self.opener.open.call_args.args[0]
        self.assertEqual(request.full_url, U.ENDPOINT)
        self.assertEqual(request.method, 'PUT')
        self.assertEqual(request.get_header('Authorization'), TOKEN)
        self.assertEqual(request.get_header('Content-type'), 'application/octet-stream')
        payload = request.data
        metadata_length, = struct.unpack('<I', payload[:4])
        metadata = json.loads(payload[4:4 + metadata_length])
        archive_length, = struct.unpack('<I', payload[4 + metadata_length:8 + metadata_length])
        trailer = payload[8 + metadata_length:]
        self.assertEqual(archive_length, len(self.data))
        self.assertEqual(trailer, self.data)
        self.assertEqual(hashlib.sha256(trailer).hexdigest(), self.artifact['sha256'])
        self.assertEqual(metadata['readme'], 'Independent test fixture README.\n')
        self.assertEqual(metadata['name'], P.NAME)
        self.assertEqual(metadata['vers'], P.VERSION)
        self.assertEqual(metadata['rust_version'], '1.97.1')
        self.assertEqual(metadata['features'], {})
        self.assertEqual([(d['name'], d['version_req']) for d in metadata['deps']], [('bytes', '^1.12.1'), ('idna', '^1.1.0'), ('prost', '^0.14.4')])
        self.assertTrue(all(d['kind'] == 'normal' and d['default_features'] and not d['optional'] and d['registry'] is None for d in metadata['deps']))
        self.release.assert_called_once_with('v2.8.0', REVISION)
        self.release_inventory.assert_called_once_with({'id': 123}, 'v2.8.0')
        self.identity.assert_called_once_with()
        self.registry.assert_called_once_with(self.artifact['sha256'])
        self.assertEqual(self.response.read_bounds, [U.MAX_RESPONSE + 1])
        self.assertTrue(any(isinstance(handler, U.NoRedirect) for handler in self.build_opener.call_args.args))
        proxy = next(handler for handler in self.build_opener.call_args.args if isinstance(handler, U.urllib.request.ProxyHandler))
        self.assertEqual(proxy.proxies, {})

    def test_each_workflow_boundary_field_required(self):
        for key in ENVIRONMENT:
            if key == 'CARGO_REGISTRY_TOKEN':
                continue
            with self.subTest(key=key), patch.dict(os.environ, {key: 'wrong'}):
                with self.assertRaises(ValueError):
                    self.upload()
        self.opener.open.assert_not_called()
        self.git.assert_not_called()

    def test_alternative_credentials_and_policy_path_refused(self):
        for name in ('CARGO_REGISTRIES_CRATES_IO_TOKEN', 'CARGO_REGISTRY_BOOTSTRAP_TOKEN'):
            with self.subTest(name=name), patch.dict(os.environ, {name: TOKEN}), self.assertRaises(ValueError):
                self.upload()
        with self.assertRaises(ValueError):
            U.upload(self.crate_path, self.directory / 'other.json')
        self.opener.open.assert_not_called()

    def test_unreviewed_checkout_prevents_upload(self):
        for outputs in ([REVISION], [WORKFLOW_REVISION, ' M policy/rust-crate-next.json']):
            with self.subTest(outputs=outputs), patch.object(P, 'git', side_effect=outputs), self.assertRaises(ValueError):
                self.upload()
        self.opener.open.assert_not_called()

    def test_pending_policy_prevents_upload(self):
        self.policy.update(status='awaiting-source-release', artifact=None)
        self.policy_path.write_text(json.dumps(self.policy))
        with self.assertRaises(ValueError):
            self.upload()
        self.opener.open.assert_not_called()

    def test_altered_crate_prevents_upload(self):
        self.crate_path.write_bytes(self.data + b'altered')
        with self.assertRaisesRegex(ValueError, 'digest'):
            self.upload()
        self.release.assert_not_called()
        self.opener.open.assert_not_called()

    def test_invalid_vcs_and_inventory_prevent_upload(self):
        self.write_fixture(vcs={'git': {'sha1': 'c' * 40}, 'path_in_vcs': 'crates/atrinik-protocol'})
        with self.assertRaisesRegex(ValueError, 'VCS'):
            self.upload()
        self.write_fixture()
        self.inventory.append('unexpected')
        with self.assertRaisesRegex(ValueError, 'inventory'):
            self.upload()
        self.opener.open.assert_not_called()

    def test_unsupported_metadata_prevents_upload(self):
        cases = [MANIFEST + b'optional=true\n', MANIFEST + b'package="other"\n',
                 MANIFEST + b'features=["extra"]\n', MANIFEST + b'registry="other"\n',
                 MANIFEST + b'[features]\nextra=[]\n', MANIFEST + b'[dev-dependencies]\n',
                 MANIFEST + b'[target."cfg(unix)".dependencies]\n',
                 MANIFEST.replace(b'edition = "2024"', b'edition = "2024"\nlinks="native"'),
                 MANIFEST.replace(b'version = "1.1.0"', b'version = "*"'),
                 MANIFEST.replace(b'readme = "README.md"', b'readme = "../secret"'),
                 MANIFEST.replace(b'description = "Test fixture"', b'description = 3'),
                 b'invalid toml']
        for manifest in cases:
            with self.subTest(manifest=manifest[-60:]):
                self.write_fixture(manifest=manifest)
                with self.assertRaises((ValueError, KeyError)):
                    self.upload()
        self.release.assert_not_called()
        self.opener.open.assert_not_called()

    def test_verified_snapshot_survives_later_file_replacement(self):
        original = self.data
        def release(*args):
            self.crate_path.write_bytes(b'changed after snapshot')
            return {'id': 123}
        self.release.side_effect = release
        self.assertEqual(self.upload(), 'success')
        self.assertTrue(self.opener.open.call_args.args[0].data.endswith(original))

    def test_release_tag_asset_or_owner_drift_prevents_upload(self):
        for mock in (self.release, self.release_asset, self.release_inventory, self.identity):
            with self.subTest(mock=mock):
                mock.side_effect = ValueError('drift')
                with self.assertRaises(ValueError):
                    self.upload()
                mock.side_effect = None
        self.release_asset.return_value = b'other archive'
        with self.assertRaisesRegex(ValueError, 'release archive'):
            self.upload()
        self.opener.open.assert_not_called()

    def test_missing_or_invalid_credentials_prevent_put(self):
        for token in ('', 'contains newline\n', 'nonascii\u00e9', 'x' * 8193):
            with self.subTest(token_length=len(token)), patch.dict(os.environ, {'CARGO_REGISTRY_TOKEN': token}), self.assertRaises(ValueError):
                self.upload()
        self.opener.open.assert_not_called()

    def test_present_is_idempotent_without_token(self):
        os.environ.pop('CARGO_REGISTRY_TOKEN')
        self.registry.return_value = 'present'
        self.assertEqual(self.upload(), 'skipped')
        self.release.assert_called_once()
        self.opener.open.assert_not_called()

    def test_registry_conflict_or_indeterminate_prevents_put(self):
        self.registry.side_effect = ValueError('checksum conflict')
        with self.assertRaises(ValueError):
            self.upload()
        self.registry.side_effect = None
        for state in ('indeterminate', 'unknown'):
            self.registry.return_value = state
            with self.assertRaises(ValueError):
                self.upload()
        self.opener.open.assert_not_called()

    def test_redirects_refused(self):
        handler = U.NoRedirect()
        for status in (301, 302, 303, 307, 308):
            with self.subTest(status=status), self.assertRaisesRegex(ValueError, 'redirect'):
                handler.redirect_request(None, None, status, '', {}, 'https://attacker.invalid/')

    def test_errors_are_redacted_and_never_retried(self):
        for error in (TimeoutError(TOKEN), urllib.error.URLError(TOKEN),
                      urllib.error.HTTPError(U.ENDPOINT, 403, TOKEN, {}, io.BytesIO(TOKEN.encode())),
                      ValueError('upload redirect refused')):
            self.opener.reset_mock()
            self.opener.open.side_effect = error
            output = io.StringIO()
            with self.subTest(error=type(error).__name__), patch.dict(os.environ, {'CARGO_REGISTRY_TOKEN': TOKEN}), patch('sys.stdout', output), patch('sys.argv', ['uploader', '--crate', str(self.crate_path)]):
                self.assertEqual(U.main(), 1)
            self.assertEqual(output.getvalue(), '{"outcome": "indeterminate"}\n')
            self.opener.open.assert_called_once()

    def test_bounded_invalid_responses_are_indeterminate(self):
        for response in (Response(b'x' * (U.MAX_RESPONSE + 1)), Response(b'not json'),
                         Response(b'[]'), Response(json.dumps({'errors': [TOKEN]}).encode()),
                         Response(b'{}', status=302), Response(b'{}', url='https://attacker.invalid/')):
            self.opener.open.return_value = response
            with self.subTest(response=response), patch.dict(os.environ, {'CARGO_REGISTRY_TOKEN': TOKEN}):
                self.assertEqual(self.upload(), 'indeterminate')

    def test_uncertain_put_can_resolve_from_public_exact_digest(self):
        self.opener.open.side_effect = TimeoutError(TOKEN)
        self.registry.side_effect = ['absent', 'indeterminate', 'present']
        self.assertEqual(self.upload(), 'success')
        self.opener.open.assert_called_once()
        self.assertEqual(self.registry.call_count, 3)
        self.assertTrue(all(call.args == (self.artifact['sha256'],) for call in self.registry.call_args_list))
        self.sleep.assert_called_once_with(5)
        self.assertEqual(self.identity.call_count, 2)

    def test_uncertain_put_resolution_is_bounded_and_fails_closed(self):
        self.opener.open.side_effect = TimeoutError(TOKEN)
        self.assertEqual(self.upload(), 'indeterminate')
        self.opener.open.assert_called_once()
        self.assertEqual(self.registry.call_count, 7)
        self.assertEqual(self.sleep.call_count, 5)
        self.registry.reset_mock(side_effect=True)
        self.registry.side_effect = ValueError('conflicting public checksum')
        self.assertEqual(U.resolve_indeterminate(self.artifact['sha256']), 'indeterminate')
        self.registry.assert_called_once()

    def test_policy_mutation_during_preflight_prevents_put(self):
        def release(*args):
            changed = dict(self.policy)
            changed.update(status='awaiting-source-release', artifact=None)
            self.policy_path.write_text(json.dumps(changed))
            return {'id': 123}
        self.release.side_effect = release
        with self.assertRaisesRegex(ValueError, 'policy changed'):
            self.upload()
        self.opener.open.assert_not_called()

    def test_oversized_manifest_or_nonregular_archive_refused(self):
        self.write_fixture(manifest=MANIFEST + b'#' + b'x' * U.MAX_METADATA)
        with self.assertRaisesRegex(ValueError, 'manifest exceeds'):
            self.upload()
        self.crate_path.unlink()
        os.mkfifo(self.crate_path)
        with self.assertRaisesRegex(ValueError, 'bounded regular file'):
            self.upload()
        self.opener.open.assert_not_called()

    def test_preflight_exception_never_prints_untrusted_text(self):
        self.release.side_effect = ValueError(TOKEN)
        output = io.StringIO()
        with patch('sys.stdout', output), patch('sys.argv', ['uploader', '--crate', str(self.crate_path)]):
            self.assertEqual(U.main(), 1)
        self.assertEqual(output.getvalue(), '{"outcome": "indeterminate"}\n')
        self.opener.open.assert_not_called()


if __name__ == '__main__':
    unittest.main()
