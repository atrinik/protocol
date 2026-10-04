#!/usr/bin/env python3
"""Check complete release coverage and reject damaged current-version artifacts."""
import hashlib
from pathlib import Path
import shutil
import sys
import tempfile


def verify(root):
    listed = {}
    for line in (root / 'SHA256SUMS').read_text().splitlines():
        digest, name = line.split('  ', 1)
        if name in listed:
            raise ValueError('duplicate checksum entry')
        listed[name] = digest
    files = {str(path.relative_to(root)) for path in root.rglob('*')
             if path.is_file() and path.name != 'SHA256SUMS'}
    if files != set(listed):
        raise ValueError('checksum inventory differs from release artifacts')
    for name, digest in listed.items():
        if hashlib.sha256((root / name).read_bytes()).hexdigest() != digest:
            raise ValueError('checksum mismatch')


def rejected(root):
    try:
        verify(root)
    except (ValueError, OSError):
        return
    raise AssertionError('damaged release passed verification')


def main():
    source = Path(sys.argv[1])
    verify(source)
    required = ('access-auth-v1.bin', 'access-tokens.md',
                'access-route-v1.schema.json', 'metaserver-directory-v2.json',
                'metaserver-directory-v2/projection.xml',
                'metaserver-game-publisher-v2.json',
                'metaserver-classic-publisher-v3.json')
    with tempfile.TemporaryDirectory(prefix='atrinik-checksum-regression-') as temporary:
        root = Path(temporary) / 'release'
        shutil.copytree(source, root)
        for name in required:
            path = root / name
            original = path.read_bytes()
            path.write_bytes(original + b'corruption')
            rejected(root)
            path.unlink()
            rejected(root)
            path.write_bytes(original)
        sums = root / 'SHA256SUMS'
        original = sums.read_text()
        sums.write_text('\n'.join(line for line in original.splitlines()
                                  if not line.endswith('  access-auth-v1.bin')) + '\n')
        rejected(root)
        sums.write_text(original)
        (root / 'unlisted-artifact').write_text('unexpected')
        rejected(root)
    print('Release checksum inventory, tamper, missing-file and omitted-entry checks passed')


if __name__ == '__main__':
    main()
