#!/usr/bin/env python3
"""Upload only the exact reviewed archive from the dedicated publication job.

Framing and metadata follow the Cargo registry API, independently implemented:
https://doc.rust-lang.org/cargo/reference/registry-web-api.html#publish
A successful response is acceptance, not public checksum verification. The
workflow must verify public registry state in a separate credential-free step.
"""
from __future__ import annotations

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import re
import stat
import struct
import sys
import tarfile
import time
import tomllib
import urllib.request
import urllib.error

# Importing the verifier must not dirty the checked-out publication source.
sys.dont_write_bytecode = True

if __package__:
    from . import crate_publication as P
else:
    import crate_publication as P

ENDPOINT = 'https://crates.io/api/v1/crates/new'
MAX_RESPONSE = 64 * 1024
MAX_METADATA = 256 * 1024


def execution_boundary(policy_path):
    """The credential-bearing stage has its own mandatory workflow boundary."""
    expected = {
        'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': 'atrinik/protocol',
        'GITHUB_REPOSITORY_ID': '1327106950', 'GITHUB_REF': 'refs/heads/main',
        'GITHUB_EVENT_NAME': 'workflow_dispatch',
        'GITHUB_WORKFLOW_REF': 'atrinik/protocol/.github/workflows/publish-crate.yml@refs/heads/main',
        'CRATE_PUBLICATION_OPERATION': 'publish',
    }
    P.require(all(os.environ.get(k) == v for k, v in expected.items()), 'unexpected upload workflow identity')
    P.require(not any(os.environ.get(k) for k in ('CARGO_REGISTRIES_CRATES_IO_TOKEN', 'CARGO_REGISTRY_BOOTSTRAP_TOKEN')), 'unexpected registry credential source')
    P.require(policy_path.resolve() == P.ROOT / 'policy/rust-crate-next.json', 'unexpected publication policy path')
    sha = os.environ.get('GITHUB_SHA', '')
    P.require(re.fullmatch(r'[0-9a-f]{40}', sha) is not None and os.environ.get('GITHUB_WORKFLOW_SHA') == sha, 'unexpected workflow revision')


def text(value):
    P.require(isinstance(value, str) and 0 < len(value) <= 8192, 'unsupported manifest text')
    return value


def strings(value, maximum):
    P.require(isinstance(value, list) and len(value) <= maximum, 'unsupported manifest list')
    return [text(item) for item in value]


def metadata(data):
    """Map the reviewed normalized manifest; fail closed on new constructs."""
    with tarfile.open(fileobj=io.BytesIO(data), mode='r:gz') as archive:
        prefix = f'{P.NAME}-{P.VERSION}/'
        raw_manifest = archive.extractfile(prefix + 'Cargo.toml').read(MAX_METADATA + 1)
        P.require(len(raw_manifest) <= MAX_METADATA, 'manifest exceeds bound')
        manifest = tomllib.loads(raw_manifest.decode())
        P.require(set(manifest) == {'package', 'lib', 'dependencies'}, 'unsupported manifest sections')
        package = manifest['package']
        P.require(isinstance(package, dict), 'unsupported package metadata')
        allowed = {'name', 'version', 'edition', 'rust-version', 'build', 'publish',
                   'autolib', 'autobins', 'autoexamples', 'autotests', 'autobenches',
                   'description', 'homepage', 'documentation', 'readme', 'keywords',
                   'license', 'repository', 'authors', 'categories'}
        P.require(set(package) <= allowed, 'unsupported package fields')
        P.require(package.get('name') == P.NAME and package.get('version') == P.VERSION
                  and package.get('publish') == ['crates-io'] and package.get('edition') == '2024', 'unexpected package identity')
        P.require(all(package.get(key) is False for key in ('build', 'autolib', 'autobins', 'autoexamples', 'autotests', 'autobenches')), 'unsupported package targets')
        P.require(manifest['lib'] == {'name': 'atrinik_protocol', 'path': 'src/lib.rs'}, 'unsupported library target')
        P.require(package.get('readme') == 'README.md' and package.get('license') == 'MIT'
                  and package.get('repository') == 'https://github.com/atrinik/protocol', 'unexpected package metadata')
        dependencies = manifest['dependencies']
        P.require(isinstance(dependencies, dict) and set(dependencies) == {'bytes', 'idna', 'prost'}, 'unsupported dependencies')
        deps = []
        for name, dependency in sorted(dependencies.items()):
            P.require(isinstance(dependency, dict) and set(dependency) == {'version'}, 'unsupported dependency fields')
            version = text(dependency['version'])
            P.require(re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+', version) is not None, 'unsupported dependency requirement')
            deps.append({'name': name, 'version_req': '^' + version, 'features': [],
                         'optional': False, 'default_features': True, 'target': None,
                         'kind': 'normal', 'registry': None, 'explicit_name_in_toml': None})
        readme = archive.extractfile(prefix + 'README.md').read(MAX_METADATA + 1)
        P.require(len(readme) <= MAX_METADATA, 'readme exceeds bound')
    result = {
        'name': P.NAME, 'vers': P.VERSION, 'deps': deps, 'features': {},
        'authors': strings(package.get('authors', []), 16),
        'description': text(package['description']),
        'documentation': text(package['documentation']) if 'documentation' in package else None,
        'homepage': text(package['homepage']) if 'homepage' in package else None,
        'readme': readme.decode(), 'readme_file': 'README.md',
        'keywords': strings(package.get('keywords', []), 5),
        'categories': strings(package.get('categories', []), 5),
        'license': 'MIT', 'license_file': None, 'repository': package['repository'],
        'badges': {}, 'links': None, 'rust_version': text(package['rust-version']),
    }
    encoded = json.dumps(result, separators=(',', ':'), ensure_ascii=True).encode()
    P.require(len(encoded) <= MAX_METADATA, 'metadata exceeds bound')
    return encoded


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise ValueError('upload redirect refused')


def put_once(data, encoded, token):
    """No retries, no redirects, and no untrusted response or exception output."""
    body = struct.pack('<I', len(encoded)) + encoded + struct.pack('<I', len(data)) + data
    request = urllib.request.Request(ENDPOINT, data=body, method='PUT', headers={
        'Authorization': token, 'Content-Type': 'application/octet-stream',
        'Accept': 'application/json',
        'User-Agent': 'atrinik-protocol-exact-upload (https://github.com/atrinik/protocol)',
    })
    # Disable environment-derived proxies; credentials go directly to the one
    # HTTPS endpoint. TLS verification retains the Python default trust store.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    try:
        with opener.open(request, timeout=30) as response:
            if response.geturl() != ENDPOINT or not 200 <= response.status < 300:
                return 'indeterminate'
            raw = response.read(MAX_RESPONSE + 1)
            if len(raw) > MAX_RESPONSE:
                return 'indeterminate'
            result = json.loads(raw)
            if not isinstance(result, dict) or 'errors' in result:
                return 'indeterminate'
            return 'success'
    except urllib.error.HTTPError as error:
        # Close the response without reading or formatting its untrusted reason.
        error.close()
        return 'indeterminate'
    except Exception:
        # Even an HTTP error can follow server-side acceptance. Recheck the
        # public registry in the credential-free verifier before any new run.
        return 'indeterminate'


def upload(crate_path, policy_path):
    execution_boundary(policy_path)
    # This dedicated boundary permits the action's short-lived token. Remove
    # it from the environment before Git subprocesses; never invoke or weaken
    # the separate credential-free preparation boundary.
    token = os.environ.pop('CARGO_REGISTRY_TOKEN', None)
    P.require(P.git('rev-parse', 'HEAD') == os.environ['GITHUB_SHA']
              and not P.git('status', '--porcelain'), 'unreviewed publication checkout')
    policy = P.load_policy(policy_path)
    P.require(policy['status'] == 'ready-for-publication', 'publication policy is not ready')
    artifact = policy['artifact']
    descriptor = os.open(crate_path, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
    with os.fdopen(descriptor, 'rb') as stream:
        info = os.fstat(stream.fileno())
        P.require(stat.S_ISREG(info.st_mode) and info.st_size <= P.MAX_BYTES, 'archive is not a bounded regular file')
        data = stream.read(P.MAX_BYTES + 1)
    P.require(len(data) <= P.MAX_BYTES and hashlib.sha256(data).hexdigest() == artifact['sha256'], 'archive digest mismatch')
    inventory = P.git('show', artifact['revision'] + ':policy/rust-crate-files.txt').splitlines()
    P.verify_crate(data, artifact['revision'], inventory)
    encoded = metadata(data)
    tag = 'v' + artifact['repository_release']
    released = P.release(tag, artifact['revision'])
    P.require(P.release_asset(released, tag, artifact['asset']) == data, 'release archive mismatch')
    P.verify_release_inventory(released, tag)
    execution_boundary(policy_path)
    P.require(P.git('rev-parse', 'HEAD') == os.environ['GITHUB_SHA']
              and not P.git('status', '--porcelain')
              and P.load_policy(policy_path) == policy, 'publication policy changed during preflight')
    P.verify_registry_identity()
    state = P.registry_state(artifact['sha256'])
    if state == 'present':
        return 'skipped'
    P.require(state == 'absent', 'registry state indeterminate')
    P.require(isinstance(token, str) and 0 < len(token) <= 8192
              and all(33 <= ord(character) <= 126 for character in token), 'missing or invalid upload credential')
    outcome = put_once(data, encoded, token)
    if outcome == 'indeterminate':
        return resolve_indeterminate(artifact['sha256'])
    return outcome


def resolve_indeterminate(digest):
    """Resolve an uncertain write with bounded reads only, never another PUT."""
    try:
        P.verify_registry_identity()
        # The caller supplies the same policy digest used for the upload.
        for attempt in range(6):
            state = P.registry_state(digest)
            if state == 'present':
                return 'success'
            if attempt < 5:
                time.sleep(5)
    except Exception:
        pass
    return 'indeterminate'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--crate', required=True, type=Path)
    parser.add_argument('--policy', type=Path, default=P.ROOT / 'policy/rust-crate-next.json')
    args = parser.parse_args()
    try:
        outcome = upload(args.crate, args.policy)
    except Exception:
        # Public artifacts, paths, HTTP errors, and credentials never become
        # diagnostics here. A failed preflight cannot authorize a PUT.
        outcome = 'indeterminate'
    print(json.dumps({'outcome': outcome}))
    return 0 if outcome in ('success', 'skipped') else 1


if __name__ == '__main__':
    raise SystemExit(main())
