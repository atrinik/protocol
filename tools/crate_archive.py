#!/usr/bin/env python3
"""Check a packaged crate archive against the reviewed inventory and sources."""
import io
import json
from pathlib import Path
import tarfile
import tomllib

NAME = 'atrinik-protocol'
MAX_BYTES = 16 * 1024 * 1024


def require(condition, message):
    if not condition:
        raise ValueError(message)


def verify_crate(path, revision, inventory, version):
    with (tarfile.open(fileobj=io.BytesIO(path), mode='r:gz') if isinstance(path, bytes) else tarfile.open(path, 'r:gz')) as archive:
        members = []
        total = 0
        for member in archive:
            total += member.size
            require(len(members) < 256 and total <= MAX_BYTES, 'crate archive exceeds bound')
            members.append(member)
        prefix = f'{NAME}-{version}/'
        names = []
        for member in members:
            require(member.isfile() and member.name.startswith(prefix), 'unexpected archive member')
            name = member.name[len(prefix):]
            require(name and '..' not in Path(name).parts and not name.startswith('/'), 'unsafe archive path')
            names.append(name)
        require(len(names) == len(set(names)) and set(names) == set(inventory), 'crate file inventory mismatch')
        def read(name):
            return archive.extractfile(prefix + name).read()
        vcs = json.loads(read('.cargo_vcs_info.json'))
        require(vcs == {'git': {'sha1': revision}, 'path_in_vcs': 'crates/atrinik-protocol'}, 'crate VCS provenance mismatch')
        manifest = tomllib.loads(read('Cargo.toml').decode())
        package = manifest['package']
        require(package['name'] == NAME and package['version'] == version and package.get('publish') == ['crates-io'], 'crate is not the publishable reviewed version')
        def no_source_override(value):
            if isinstance(value, dict):
                require('path' not in value and 'git' not in value, 'unapproved crate dependency source')
                for child in value.values():
                    no_source_override(child)
            elif isinstance(value, list):
                for child in value:
                    no_source_override(child)
        for key in ('dependencies', 'dev-dependencies', 'build-dependencies', 'target'):
            no_source_override(manifest.get(key, {}))
        lock = tomllib.loads(read('Cargo.lock').decode())
        for package in lock['package']:
            if package['name'] == NAME and package['version'] == version:
                require('source' not in package, 'unexpected root crate source')
            else:
                require(package.get('source') == 'registry+https://github.com/rust-lang/crates.io-index', 'unapproved locked dependency source')
