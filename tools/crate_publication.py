#!/usr/bin/env python3
"""Credential-free preparation and verification; this program never uploads."""
from __future__ import annotations

import argparse
import hashlib
import json
import io
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import time
import tomllib
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
NAME = 'atrinik-protocol'
VERSION = '0.2.0'
ASSET = f'{NAME}-{VERSION}.crate'
MAX_BYTES = 16 * 1024 * 1024
API = 'https://api.github.com/repos/atrinik/protocol'


def require(condition, message):
    if not condition:
        raise ValueError(message)


def load_policy(path):
    policy = json.loads(Path(path).read_text())
    require(set(policy) == {'schema_version', 'name', 'version', 'status', 'artifact'}, 'unexpected policy fields')
    require(policy['schema_version'] == 1 and policy['name'] == NAME and policy['version'] == VERSION, 'unexpected crate identity')
    if policy['status'] == 'awaiting-source-release':
        require(policy['artifact'] is None, 'pending policy must not invent artifact pins')
    else:
        require(policy['status'] == 'ready-for-publication', 'unknown publication state')
        validate_artifact(policy['artifact'])
    return policy


def validate_artifact(value):
    require(isinstance(value, dict) and set(value) == {'repository_release', 'revision', 'asset', 'sha256'}, 'incomplete artifact pins')
    require(re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+', value['repository_release']) is not None, 'invalid release version')
    require(re.fullmatch(r'[0-9a-f]{40}', value['revision']) is not None, 'invalid source revision')
    require(value['asset'] == ASSET and re.fullmatch(r'[0-9a-f]{64}', value['sha256']) is not None, 'invalid artifact identity')


def execution_boundary():
    require(not any(os.environ.get(key) for key in ('CARGO_REGISTRY_TOKEN', 'CARGO_REGISTRIES_CRATES_IO_TOKEN', 'CARGO_REGISTRY_BOOTSTRAP_TOKEN')), 'preparation must run without registry credentials')
    if os.environ.get('GITHUB_ACTIONS') == 'true':
        expected = {'GITHUB_REPOSITORY': 'atrinik/protocol', 'GITHUB_REPOSITORY_ID': '1327106950',
                    'GITHUB_REF': 'refs/heads/main', 'GITHUB_EVENT_NAME': 'workflow_dispatch',
                    'GITHUB_WORKFLOW_REF': 'atrinik/protocol/.github/workflows/publish-crate.yml@refs/heads/main'}
        require(all(os.environ.get(k) == v for k, v in expected.items()), 'unexpected publication workflow identity')


class SafeRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        from urllib.parse import urlparse
        parsed = urlparse(newurl)
        require(parsed.scheme == 'https' and parsed.hostname in {'github.com', 'release-assets.githubusercontent.com', 'objects.githubusercontent.com', 'crates.io', 'index.crates.io'}, 'unexpected download redirect')
        return super().redirect_request(req, fp, code, msg, headers, newurl)


def download(url, absent=False):
    request = urllib.request.Request(url, headers={'User-Agent': 'atrinik-protocol-release-verifier (https://github.com/atrinik/protocol)', 'Accept': 'application/json'})
    try:
        with urllib.request.build_opener(SafeRedirect).open(request, timeout=20) as response:
            data = response.read(MAX_BYTES + 1)
            require(len(data) <= MAX_BYTES, 'response exceeds bound')
            return data
    except urllib.error.HTTPError as error:
        if absent and error.code == 404:
            return None
        raise


def git(*arguments, cwd=ROOT):
    return subprocess.check_output(['git', *arguments], cwd=cwd, text=True, timeout=60).strip()


def release(tag, revision):
    require(re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+', tag) is not None, 'invalid source tag')
    require(re.fullmatch(r'[0-9a-f]{40}', revision) is not None, 'invalid source revision')
    require(git('rev-parse', '--verify', f'{tag}^{{commit}}') == revision, 'source tag drift')
    subprocess.run(['git', 'merge-base', '--is-ancestor', revision, 'origin/main'], cwd=ROOT, check=True, timeout=60)
    remote = json.loads(download(f'{API}/git/ref/tags/{tag}'))['object']
    for _ in range(2):
        if remote['type'] != 'tag':
            break
        remote = json.loads(download(f"{API}/git/tags/{remote['sha']}"))['object']
    require(remote['type'] == 'commit' and remote['sha'] == revision, 'remote source tag drift')
    value = json.loads(download(f'{API}/releases/tags/{tag}'))
    require(value['tag_name'] == tag and not value['draft'] and not value['prerelease'] and value.get('published_at'), 'source release is not published')
    provenance = json.loads(release_asset(value, tag, 'provenance.json'))
    require(provenance['revision'] == revision and provenance['version'] == tag[1:], 'release provenance drift')
    return value


def release_asset(value, tag, name):
    assets = [asset for asset in value['assets'] if asset['name'] == name]
    require(len(assets) == 1 and assets[0]['state'] == 'uploaded', 'missing or duplicate release asset')
    asset = assets[0]
    url = f'https://github.com/atrinik/protocol/releases/download/{tag}/{name}'
    require(asset['browser_download_url'] == url and asset['size'] <= MAX_BYTES, 'unexpected release asset location or bound')
    data = download(url)
    digest = 'sha256:' + hashlib.sha256(data).hexdigest()
    require(asset.get('digest') == digest and len(data) == asset['size'], 'release API asset digest mismatch')
    return data


def verify_release_inventory(value, tag):
    assets = value['assets']
    require(isinstance(assets, list) and 1 <= len(assets) <= 128,
            'release asset inventory exceeds bound')
    expected = {}
    for asset in assets:
        name = asset['name']
        require(re.fullmatch(r'[A-Za-z0-9_.+-]+', name) is not None and
                name not in expected and asset.get('state') == 'uploaded' and
                isinstance(asset.get('size'), int) and 0 < asset['size'] <= MAX_BYTES,
                'unsafe or duplicate release asset')
        digest = asset.get('digest', '')
        require(re.fullmatch(r'sha256:[0-9a-f]{64}', digest) is not None,
                'release asset digest is missing')
        expected[name] = digest[7:]
    manifest = release_asset(value, tag, 'SHA256SUMS')
    require(len(manifest) <= 32768, 'release checksum manifest exceeds bound')
    listed = {}
    for line in manifest.decode('ascii').splitlines():
        match = re.fullmatch(r'([0-9a-f]{64})  ([A-Za-z0-9_.+-]+)', line)
        require(match is not None, 'malformed release checksum entry')
        digest, name = match.groups()
        require(name not in listed and name != 'SHA256SUMS', 'duplicate or recursive release checksum')
        listed[name] = digest
    del expected['SHA256SUMS']
    require(listed == expected, 'release checksums do not cover exact asset inventory')


def verify_crate(path, revision, inventory):
    with (tarfile.open(fileobj=io.BytesIO(path), mode='r:gz') if isinstance(path, bytes) else tarfile.open(path, 'r:gz')) as archive:
        members = []
        total = 0
        for member in archive:
            total += member.size
            require(len(members) < 256 and total <= MAX_BYTES, 'crate archive exceeds bound')
            members.append(member)
        prefix = f'{NAME}-{VERSION}/'
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
        require(package['name'] == NAME and package['version'] == VERSION and package.get('publish') == ['crates-io'], 'crate is not the publishable reviewed version')
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
            if package['name'] == NAME and package['version'] == VERSION:
                require('source' not in package, 'unexpected root crate source')
            else:
                require(package.get('source') == 'registry+https://github.com/rust-lang/crates.io-index', 'unapproved locked dependency source')


def verify_registry_identity():
    package = json.loads(download(f'https://crates.io/api/v1/crates/{NAME}'))['crate']
    require(package.get('id') == NAME and package.get('name') == NAME and
            package.get('repository') == 'https://github.com/atrinik/protocol',
            'public registry package identity changed')
    owners = json.loads(download(f'https://crates.io/api/v1/crates/{NAME}/owners'))['users']
    require(isinstance(owners, list) and 1 <= len(owners) <= 100 and any(
        owner.get('kind') == 'user' and owner.get('id') == 437663 and
        owner.get('login') == 'zoeyrose' and owner.get('github_username_matches') is True
        for owner in owners), 'public registry owner identity changed')


def registry_state(digest):
    api = download(f'https://crates.io/api/v1/crates/{NAME}/{VERSION}', absent=True)
    index = download(f'https://index.crates.io/at/ri/{NAME}', absent=True)
    version = None if api is None else json.loads(api)['version']
    if version is not None:
        require(version.get('crate') == NAME and version.get('num') == VERSION and
                version.get('yanked') is False, 'registry version identity or availability changed')
    api_digest = None if version is None else version['checksum']
    entries = [] if index is None else [json.loads(line) for line in index.splitlines() if line]
    matches = [entry for entry in entries if entry['vers'] == VERSION]
    require(len(matches) <= 1, 'duplicate registry index version')
    require(not matches or (matches[0].get('name') == NAME and matches[0].get('yanked') is False), 'registry index identity or availability changed')
    index_digest = None if not matches else matches[0]['cksum']
    require(all(value in (None, digest) for value in (api_digest, index_digest)), 'registry checksum conflict; never republish')
    if api_digest == digest and index_digest == digest:
        return 'present'
    if api_digest is None and index_digest is None:
        return 'absent'
    return 'indeterminate'


def prepare(tag, revision, output, policy, publishing=False):
    require(not any(char in str(output) for char in '\r\n'), 'invalid output path')
    require(not output.exists() and not output.is_relative_to(ROOT), 'output must be absent and outside source checkout')
    value = release(tag, revision)
    require(subprocess.check_output(['rustc', '+1.97.1', '--version'], text=True, timeout=60).startswith('rustc 1.97.1 '), 'wrong Rust toolchain')
    output.mkdir(parents=True)
    source = output / 'source'
    subprocess.run(['git', 'worktree', 'add', '--detach', str(source), revision], cwd=ROOT, check=True, timeout=60)
    require(not git('status', '--porcelain', cwd=source), 'dirty source worktree')
    manifest = tomllib.loads((source / 'crates/atrinik-protocol/Cargo.toml').read_text())
    require(manifest['package'].get('publish') == ['crates-io'], 'source release has publication disabled; prepare a reviewed publishable source release first')
    inventory = (source / 'policy/rust-crate-files.txt').read_text().splitlines()
    packages = []
    for number in (1, 2):
        target = output / f'package-{number}'
        subprocess.run(['cargo', '+1.97.1', 'package', '--locked', '--manifest-path', str(source / 'crates/atrinik-protocol/Cargo.toml'), '--target-dir', str(target)], cwd=source, check=True, timeout=600)
        package = target / 'package' / ASSET
        verify_crate(package, revision, inventory)
        packages.append(package)
    require(not git('status', '--porcelain', cwd=source), 'packaging changed source')
    first = packages[0].read_bytes()
    require(first == packages[1].read_bytes(), 'crate reproduction mismatch')
    digest = hashlib.sha256(first).hexdigest()
    artifact = {'repository_release': tag[1:], 'revision': revision, 'asset': ASSET, 'sha256': digest}
    ready = False
    if publishing:
        require(artifact == policy['artifact'], 'reproduced artifact differs from reviewed pins')
        require(release_asset(value, tag, ASSET) == first, 'published release asset differs from reproduced crate')
        verify_release_inventory(value, tag)
        verify_registry_identity()
        state = registry_state(digest)
        require(state != 'indeterminate', 'registry result indeterminate; re-read before any retry')
        ready = state == 'absent'
    # Re-read public release and local ref after expensive preparation.
    current = release(tag, revision)
    require(current['id'] == value['id'], 'release identity changed during preparation')
    if publishing:
        require(release_asset(current, tag, ASSET) == first, 'release asset changed during preparation')
        verify_release_inventory(current, tag)
    shutil.copyfile(packages[0], output / ASSET)
    (output / 'artifact.json').write_text(json.dumps(artifact, indent=2) + '\n')
    if os.environ.get('GITHUB_OUTPUT'):
        with open(os.environ['GITHUB_OUTPUT'], 'a') as stream:
            stream.write(f'ready_for_upload={str(ready).lower()}\nsource_directory={source}\n')
    print(json.dumps({'artifact': artifact, 'ready_for_upload': ready, 'github_release_immutable': current.get('immutable', False)}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('operation', choices=('prepare', 'verify-publish', 'verify-registry'))
    parser.add_argument('--policy', type=Path, default=ROOT / 'policy/rust-crate-next.json')
    parser.add_argument('--source-tag')
    parser.add_argument('--source-revision')
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    execution_boundary()
    policy = load_policy(args.policy)
    if args.operation == 'prepare':
        require(args.source_tag is not None and args.source_revision is not None and args.output is not None, 'prepare requires exact source coordinates and output')
        prepare(args.source_tag, args.source_revision, args.output.resolve(), policy)
    else:
        require(policy['status'] == 'ready-for-publication', 'publication disabled until actual source-release artifact pins are reviewed')
        artifact = policy['artifact']
        if args.operation == 'verify-publish':
            require(args.output is not None, 'verification output required')
            require(args.source_tag == 'v' + artifact['repository_release'] and args.source_revision == artifact['revision'], 'publication inputs differ from reviewed artifact pins')
            prepare('v' + artifact['repository_release'], artifact['revision'], args.output.resolve(), policy, True)
        else:
            for attempt in range(6):
                verify_registry_identity()
                if registry_state(artifact['sha256']) == 'present':
                    print('Public registry API and sparse-index checksums match reviewed artifact')
                    return
                if attempt < 5:
                    time.sleep(5)
            raise ValueError('registry verification indeterminate; inspect public state before any retry')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, TypeError, OSError, subprocess.SubprocessError) as error:
        raise SystemExit(f'crate verification failed: {error}') from error
