#!/usr/bin/env python3
"""Reconstruct the private Phase 10O build input from the Phase 10N snapshot."""

import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import stat
import tarfile

checkout = Path('/home/bts/Projects/try-omarchy-linux-core')
parent = Path('/home/bts/.cache/try-omarchy-phase10n-source')
current = Path('/home/bts/.cache/try-omarchy-phase10o-source')
output = checkout / 'docs/evidence/phase10o'


def inventory(root):
    result = {}
    for path in root.rglob('*'):
        if path.is_file() or path.is_symlink():
            name = path.relative_to(root).as_posix()
            data = os.fsencode(os.readlink(path)) if path.is_symlink() else path.read_bytes()
            result[name] = hashlib.sha256(data).hexdigest()
    return result


before = inventory(parent)
after = inventory(current)
changed = {name: digest for name, digest in sorted(after.items()) if before.get(name) != digest}
removed = sorted(set(before) - set(after))
manifest = {
    'starting_head': 'e9d8ecd9d9d6576402becb8fa97c16e46edd89ae',
    'parent_source_delta_sha256': hashlib.sha256((checkout / 'docs/evidence/phase10n/source-delta.tar.gz').read_bytes()).hexdigest(),
    'parent_bundle_sha256': hashlib.sha256((checkout / 'docs/evidence/phase10n/phase10n-portal-backup-final.flatpak').read_bytes()).hexdigest(),
    'parent_source': str(parent),
    'build_source': str(current),
    'parent_files': len(before),
    'current_files': len(after),
    'changed_or_added': changed,
    'removed': removed,
}
data = (json.dumps(manifest, sort_keys=True, indent=2) + '\n').encode()
output.mkdir(exist_ok=True)
(output / 'source-delta.json').write_bytes(data)
with (output / 'source-delta.tar.gz').open('wb') as raw:
    with gzip.GzipFile(filename='', mode='wb', fileobj=raw, mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode='w') as archive:
            info = tarfile.TarInfo('SOURCE-DELTA.json')
            info.mode = 0o644
            info.mtime = 0
            info.size = len(data)
            archive.addfile(info, io.BytesIO(data))
            for name in changed:
                path = current / name
                info = tarfile.TarInfo(name)
                info.mode = stat.S_IMODE(path.lstat().st_mode)
                info.mtime = 0
                if path.is_symlink():
                    info.type = tarfile.SYMTYPE
                    info.linkname = os.readlink(path)
                    archive.addfile(info)
                else:
                    content = path.read_bytes()
                    info.size = len(content)
                    archive.addfile(info, io.BytesIO(content))
print(f'{len(changed)} changed or added, {len(removed)} removed, {len(after)} current')
