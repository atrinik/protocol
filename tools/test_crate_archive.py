#!/usr/bin/env python3
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location('crate_archive', Path(__file__).with_name('crate_archive.py'))
A = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(A)
REVISION = 'a' * 40
VERSION = '9.8.7'


class CrateArchiveTests(unittest.TestCase):
    def test_crate_identity_inventory_and_dependency_checks(self):
        entries = {
            '.cargo_vcs_info.json': json.dumps({'git': {'sha1': REVISION}, 'path_in_vcs': 'crates/atrinik-protocol'}).encode(),
            'Cargo.toml': f'[package]\nname="atrinik-protocol"\nversion="{VERSION}"\npublish=["crates-io"]\n'.encode(),
            'Cargo.lock': f'version=4\n[[package]]\nname="atrinik-protocol"\nversion="{VERSION}"\n'.encode(),
        }
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / f'{A.NAME}-{VERSION}.crate'
            def write(data, version=VERSION):
                with tarfile.open(path, 'w:gz') as archive:
                    for name, content in data.items():
                        member = tarfile.TarInfo(f'{A.NAME}-{version}/{name}')
                        member.size = len(content)
                        archive.addfile(member, io.BytesIO(content))
            write(entries)
            A.verify_crate(path, REVISION, entries, VERSION)
            A.verify_crate(path.read_bytes(), REVISION, entries, VERSION)
            with self.assertRaisesRegex(ValueError, 'unexpected archive member'):
                A.verify_crate(path, REVISION, entries, '9.8.6')
            for name, replacement, error in [
                ('.cargo_vcs_info.json', b'{"git":{"sha1":"wrong"}}', 'VCS'),
                ('Cargo.toml', entries['Cargo.toml'].replace(b'["crates-io"]', b'false'), 'publishable'),
                ('Cargo.toml', entries['Cargo.toml'] + b'[dependencies.x]\npath="../x"\n', 'dependency'),
                ('Cargo.lock', entries['Cargo.lock'] + b'[[package]]\nname="x"\nversion="1"\nsource="git+evil"\n', 'dependency'),
            ]:
                write({**entries, name: replacement})
                with self.subTest(name=name, error=error), self.assertRaisesRegex(ValueError, error):
                    A.verify_crate(path, REVISION, entries, VERSION)
            write({**entries, 'unexpected': b'x'})
            with self.assertRaisesRegex(ValueError, 'inventory'):
                A.verify_crate(path, REVISION, entries, VERSION)


if __name__ == '__main__':
    unittest.main()
