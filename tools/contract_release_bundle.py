#!/usr/bin/env python3
"""Build the deterministic release bundle for downloadable contract assets."""

import gzip
import hashlib
import io
import os
from pathlib import Path, PurePosixPath
import re
import sys
import tarfile


TOP_LEVEL_FILES = (
    "LICENSE",
    "THIRD_PARTY_NOTICES.md",
    "access-auth-v1.bin",
    "access-client-hello-v1.tsv",
    "access-resolve-v1.json",
    "access-resolve-v1.schema.json",
    "access-route-bounds-v1.json",
    "access-route-state-v1.json",
    "access-route-v1.schema.json",
    "access-routes-v1.json",
    "access-tokens-v1.json",
    "access-tokens.md",
    "atrinik-game-v1.binpb",
    "framing.json",
    "metaserver-classic-publisher-v2.json",
    "metaserver-classic-publisher-v2.schema.json",
    "metaserver-classic-publisher-v3.json",
    "metaserver-classic-publisher-v3.schema.json",
    "metaserver-directory-v1.json",
    "metaserver-directory-v1.schema.json",
    "metaserver-directory-v2.json",
    "metaserver-directory-v2.schema.json",
    "metaserver-directory.md",
    "metaserver-game-publisher-v1.json",
    "metaserver-game-publisher-v1.schema.json",
    "metaserver-game-publisher-v2.json",
    "metaserver-game-publisher-v2.schema.json",
    "metaserver-publisher-v1.json",
    "metaserver-publisher.md",
)
NESTED_ROOTS = ("metaserver-directory-v1", "metaserver-directory-v2")
NESTED_FILES = (
    "canonical.json",
    "negative-duplicate-server.json",
    "negative-expired-at-generation.json",
    "negative-identity-mismatch.json",
    "negative-invalid-alabel.json",
    "negative-noncanonical-whitespace.json",
    "negative-numeric-endpoint.json",
    "negative-private-field.json",
    "negative-status-count.json",
    "negative-unordered-servers.json",
    "negative-unsupported-schema.json",
    "negative-xml-noncharacter.json",
    "negative-zero-generation.json",
    "projection-semantics.json",
    "projection.xml",
)
PAYLOAD_PATHS = frozenset(TOP_LEVEL_FILES) | frozenset(
    f"{root}/{name}" for root in NESTED_ROOTS for name in NESTED_FILES
)
MAX_FILES = 128
MAX_TOTAL_BYTES = 64 * 1024 * 1024
VERSION_PATTERN = re.compile(r"[0-9A-Za-z][0-9A-Za-z.+-]*")


def _read_payloads(output: Path) -> dict[str, bytes]:
    if output.is_symlink() or not output.is_dir():
        raise ValueError("release output must be a real directory")
    if len(PAYLOAD_PATHS) > MAX_FILES:
        raise ValueError("contract bundle exceeds its file-count bound")
    payloads = {}
    total = 0
    for relative in sorted(PAYLOAD_PATHS):
        path = output / relative
        if path.is_symlink() or not path.is_file():
            raise ValueError(f"missing or unsafe contract asset: {relative}")
        size = path.stat().st_size
        total += size
        if total > MAX_TOTAL_BYTES:
            raise ValueError("contract bundle exceeds its byte bound")
        data = path.read_bytes()
        if len(data) != size:
            raise ValueError(f"contract asset changed while reading: {relative}")
        payloads[relative] = data

    for root_name in NESTED_ROOTS:
        root = output / root_name
        discovered = set()
        for path in root.rglob("*"):
            if path.is_symlink() or not path.is_file():
                if path.is_dir() and not path.is_symlink():
                    continue
                raise ValueError(f"unsafe nested contract asset: {path}")
            discovered.add(path.relative_to(output).as_posix())
        expected = {name for name in PAYLOAD_PATHS if name.startswith(root_name + "/")}
        if discovered != expected:
            raise ValueError(f"unexpected {root_name} contract inventory")
    return payloads


def _tar_info(name: str, *, directory: bool, size: int = 0) -> tarfile.TarInfo:
    info = tarfile.TarInfo(name + ("/" if directory else ""))
    info.type = tarfile.DIRTYPE if directory else tarfile.REGTYPE
    info.mode = 0o755 if directory else 0o644
    info.uid = 0
    info.gid = 0
    info.uname = ""
    info.gname = ""
    info.mtime = 0
    info.size = size
    return info


def build_bundle(output: Path, version: str) -> Path:
    if VERSION_PATTERN.fullmatch(version) is None:
        raise ValueError("invalid release version")
    payloads = _read_payloads(output)
    checksums = "".join(
        f"{hashlib.sha256(payloads[name]).hexdigest()}  {name}\n"
        for name in sorted(payloads)
    ).encode("ascii")
    archive_root = f"atrinik-protocol-contracts-{version}"
    members = dict(payloads)
    members["SHA256SUMS"] = checksums

    directories = {archive_root}
    for relative in members:
        parent = PurePosixPath(archive_root, relative).parent
        while parent.as_posix() != ".":
            directories.add(parent.as_posix())
            if parent.as_posix() == archive_root:
                break
            parent = parent.parent

    destination = output / f"{archive_root}.tar.gz"
    temporary = output / f".{archive_root}.tar.gz.tmp"
    if destination.exists() or temporary.exists():
        raise ValueError("contract bundle output already exists")
    try:
        with temporary.open("xb") as raw:
            with gzip.GzipFile(fileobj=raw, mode="wb", filename="", mtime=0) as compressed:
                with tarfile.open(
                    fileobj=compressed, mode="w", format=tarfile.GNU_FORMAT
                ) as archive:
                    for name in sorted(directories):
                        archive.addfile(_tar_info(name, directory=True))
                    for relative in sorted(members):
                        data = members[relative]
                        archive.addfile(
                            _tar_info(
                                f"{archive_root}/{relative}",
                                directory=False,
                                size=len(data),
                            ),
                            io.BytesIO(data),
                        )
        os.replace(temporary, destination)
    finally:
        temporary.unlink(missing_ok=True)
    return destination


def main() -> None:
    if len(sys.argv) != 3:
        raise SystemExit(f"usage: {sys.argv[0]} OUTPUT VERSION")
    try:
        build_bundle(Path(sys.argv[1]), sys.argv[2])
    except (OSError, ValueError) as error:
        raise SystemExit(f"contract bundle failed: {error}") from error


if __name__ == "__main__":
    main()
