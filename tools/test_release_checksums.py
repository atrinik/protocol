#!/usr/bin/env python3
"""Check flat release discovery and both levels of release checksums."""

import copy
import gzip
import hashlib
import io
from pathlib import Path, PurePosixPath
import re
import shutil
import sys
import tarfile
import tempfile

import contract_release_bundle as bundle_spec


CHECKSUM_LINE = re.compile(r"([0-9a-f]{64})  ([A-Za-z0-9_.+/-]+)")
MAX_BUNDLE_BYTES = bundle_spec.MAX_TOTAL_BYTES + 1024 * 1024


def file_sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        while chunk := stream.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()


def parse_checksums(data, *, nested):
    listed = {}
    try:
        lines = data.decode("ascii").splitlines()
    except UnicodeDecodeError as error:
        raise ValueError("checksum inventory is not ASCII") from error
    if not lines:
        raise ValueError("empty checksum inventory")
    for line in lines:
        match = CHECKSUM_LINE.fullmatch(line)
        if match is None:
            raise ValueError("malformed checksum entry")
        digest, name = match.groups()
        path = PurePosixPath(name)
        if path.is_absolute() or ".." in path.parts or "\\" in name:
            raise ValueError("unsafe checksum path")
        if not nested and len(path.parts) != 1:
            raise ValueError("outer checksum path is not flat")
        if name == "SHA256SUMS" or name in listed:
            raise ValueError("recursive or duplicate checksum entry")
        listed[name] = digest
    return listed


def verify_bundle(path):
    if path.stat().st_size > MAX_BUNDLE_BYTES:
        raise ValueError("contract bundle exceeds its compressed-size bound")
    with path.open("rb") as stream:
        header = stream.read(10)
    if (len(header) < 10 or header[:2] != b"\x1f\x8b" or
            header[4:8] != b"\0\0\0\0"):
        raise ValueError("contract bundle has a nondeterministic gzip header")
    if header[3] & 0x08:
        raise ValueError("contract bundle records a gzip file name")

    archive_root = path.name.removesuffix(".tar.gz")
    prefix = archive_root + "/"
    payloads = {}
    names = set()
    directories = set()
    total = 0
    with tarfile.open(path, mode="r:gz") as archive:
        maximum_members = (
            bundle_spec.MAX_FILES + len(bundle_spec.NESTED_ROOTS) + 2
        )
        for index, member in enumerate(archive):
            if index >= maximum_members:
                raise ValueError("contract bundle exceeds its member-count bound")
            if member.name in names:
                raise ValueError("duplicate contract bundle member")
            names.add(member.name)
            pure = PurePosixPath(member.name)
            if pure.is_absolute() or ".." in pure.parts or "\\" in member.name:
                raise ValueError("unsafe contract bundle path")
            if not (member.name == archive_root or member.name.startswith(prefix)):
                raise ValueError("contract bundle has multiple roots")
            if (member.mtime != 0 or member.uid != 0 or member.gid != 0 or
                    member.uname or member.gname):
                raise ValueError("contract bundle metadata is not normalized")
            if member.isdir():
                if member.mode != 0o755:
                    raise ValueError("contract bundle directory mode differs")
                directories.add(member.name.rstrip("/"))
                continue
            if not member.isfile() or member.mode != 0o644:
                raise ValueError("unsafe contract bundle member type")
            total += member.size
            if total > MAX_BUNDLE_BYTES:
                raise ValueError("contract bundle exceeds its expanded-size bound")
            relative = member.name[len(prefix):]
            stream = archive.extractfile(member)
            if stream is None:
                raise ValueError("contract bundle member is unreadable")
            data = stream.read(member.size + 1)
            if len(data) != member.size:
                raise ValueError("contract bundle member size differs")
            payloads[relative] = data

    expected = set(bundle_spec.PAYLOAD_PATHS) | {"SHA256SUMS"}
    if set(payloads) != expected:
        raise ValueError("contract bundle inventory differs")
    expected_directories = {archive_root} | {
        f"{archive_root}/{name}" for name in bundle_spec.NESTED_ROOTS
    }
    if directories != expected_directories:
        raise ValueError("contract bundle directory inventory differs")
    listed = parse_checksums(payloads["SHA256SUMS"], nested=True)
    if set(listed) != set(bundle_spec.PAYLOAD_PATHS):
        raise ValueError("internal checksum inventory differs")
    for name, digest in listed.items():
        if hashlib.sha256(payloads[name]).hexdigest() != digest:
            raise ValueError("internal checksum mismatch")


def verify(root):
    entries = list(root.iterdir())
    if any(not path.is_file() or path.is_symlink() for path in entries):
        raise ValueError("release contains a non-downloadable or unsafe entry")
    discovered = {path.name for path in entries}
    semantic_release_discovery = {
        path.name for path in root.glob("*") if path.is_file()
    }
    if semantic_release_discovery != discovered:
        raise ValueError("release contains an asset missed by the publish glob")
    if "SHA256SUMS" not in discovered:
        raise ValueError("release checksum inventory is missing")
    required = set(bundle_spec.TOP_LEVEL_FILES) | {
        "SHA256SUMS", "provenance.json", "sbom.cdx.json"
    }
    if not required.issubset(discovered):
        raise ValueError("release is missing a required flat asset")
    source_archives = {
        name for name in discovered
        if name.startswith("atrinik-protocol-") and name.endswith(".tar.gz")
        and not name.startswith("atrinik-protocol-contracts-")
    }
    if len(source_archives) != 1:
        raise ValueError("release must contain one source archive")
    listed = parse_checksums((root / "SHA256SUMS").read_bytes(), nested=False)
    if set(listed) != discovered - {"SHA256SUMS"}:
        raise ValueError("checksum inventory differs from downloadable assets")
    for name, digest in listed.items():
        if file_sha256(root / name) != digest:
            raise ValueError("outer checksum mismatch")

    bundles = sorted(root.glob("atrinik-protocol-contracts-*.tar.gz"))
    if len(bundles) != 1 or bundles[0].name not in listed:
        raise ValueError("release must contain one checksummed contract bundle")
    verify_bundle(bundles[0])


def rejected(root):
    try:
        verify(root)
    except (ValueError, OSError, tarfile.TarError):
        return
    raise AssertionError("damaged release passed verification")


def refresh_outer_bundle_checksum(root, bundle):
    sums = root / "SHA256SUMS"
    replacement = file_sha256(bundle)
    lines = sums.read_text().splitlines()
    matches = [index for index, line in enumerate(lines)
               if line.endswith("  " + bundle.name)]
    if len(matches) != 1:
        raise AssertionError("test fixture has no unique bundle checksum")
    lines[matches[0]] = f"{replacement}  {bundle.name}"
    sums.write_text("\n".join(lines) + "\n")


def rewrite_bundle_member(bundle, relative, replacement):
    archive_root = bundle.name.removesuffix(".tar.gz")
    target = f"{archive_root}/{relative}"
    records = []
    found = False
    with tarfile.open(bundle, mode="r:gz") as source:
        for member in source.getmembers():
            data = source.extractfile(member).read() if member.isfile() else None
            if member.name == target:
                found = True
                if replacement is None:
                    continue
                data = replacement
                member = copy.copy(member)
                member.size = len(data)
            records.append((copy.copy(member), data))
    if not found:
        raise AssertionError("test bundle member is missing")
    with bundle.open("wb") as raw:
        with gzip.GzipFile(fileobj=raw, mode="wb", filename="", mtime=0) as compressed:
            with tarfile.open(fileobj=compressed, mode="w",
                              format=tarfile.GNU_FORMAT) as destination:
                for member, data in records:
                    destination.addfile(member, None if data is None else io.BytesIO(data))


def main():
    source = Path(sys.argv[1])
    verify(source)
    required = (
        "access-auth-v1.bin",
        "access-tokens.md",
        "access-route-v1.schema.json",
        "metaserver-directory-v2.json",
        "metaserver-game-publisher-v2.json",
        "metaserver-classic-publisher-v3.json",
        "provenance.json",
        "sbom.cdx.json",
    )
    with tempfile.TemporaryDirectory(prefix="atrinik-checksum-regression-") as temporary:
        temporary = Path(temporary)

        for name in required:
            root = temporary / ("missing-" + name.replace(".", "-"))
            shutil.copytree(source, root)
            path = root / name
            path.write_bytes(path.read_bytes() + b"corruption")
            rejected(root)
            path.unlink()
            rejected(root)

        root = temporary / "missing-bundle"
        shutil.copytree(source, root)
        next(root.glob("atrinik-protocol-contracts-*.tar.gz")).unlink()
        rejected(root)

        root = temporary / "omitted-checksum"
        shutil.copytree(source, root)
        sums = root / "SHA256SUMS"
        original = sums.read_text()
        sums.write_text("\n".join(
            line for line in original.splitlines()
            if not line.endswith("  access-auth-v1.bin")) + "\n")
        rejected(root)

        root = temporary / "extra-asset"
        shutil.copytree(source, root)
        (root / "unlisted-artifact").write_text("unexpected")
        rejected(root)

        root = temporary / "nested-output"
        shutil.copytree(source, root)
        (root / "not-uploaded").mkdir()
        (root / "not-uploaded" / "fixture").write_text("missed by flat glob")
        rejected(root)

        nested = "metaserver-directory-v2/projection.xml"
        for label, replacement in (("missing-nested", None),
                                   ("tampered-nested", b"corruption")):
            root = temporary / label
            shutil.copytree(source, root)
            bundle = next(root.glob("atrinik-protocol-contracts-*.tar.gz"))
            rewrite_bundle_member(bundle, nested, replacement)
            refresh_outer_bundle_checksum(root, bundle)
            rejected(root)

    print("Flat release discovery and outer/internal checksum regressions passed")


if __name__ == "__main__":
    main()
