#!/usr/bin/env python3
"""Package the exact Phase 10D devbox build input against Phase 9B."""

import gzip
import hashlib
import io
import json
import os
import pathlib
import stat
import subprocess
import tarfile

checkout = pathlib.Path('/home/bts/Projects/try-omarchy-linux-core')
snapshot = pathlib.Path('/home/bts/.cache/try-omarchy-phase10d-source')
out = checkout / 'docs/evidence/phase10c'
base = pathlib.Path('/data/try-omarchy-linux-spike')
sources = [
    base / 'sol-handoff-2026-09-26/source-sha256.json',
    base / 'sol-phase8/source-delta.json',
    base / 'sol-phase8b/source-delta.json',
    base / 'sol-phase8b-correction/source-delta.json',
    base / 'sol-phase9-slice/source-delta.json',
    base / 'sol-release-continuation/review-closure/source-delta.json',
    base / 'sol-release-continuation/phase9b-controls/source-delta.json',
]
prior = json.loads(sources[0].read_text())
for source in sources[1:]:
    delta = json.loads(source.read_text())
    prior.update(delta['changed_or_added'])
    for name in delta['removed']:
        prior.pop(name, None)

raw = subprocess.check_output(['git', 'ls-files', '-c', '-o', '--exclude-standard', '-z'], cwd=checkout)
names = sorted({os.fsdecode(name) for name in raw.split(b'\0') if name and not os.fsdecode(name).startswith('docs/evidence/phase10c/')})
current = {}
for name in names:
    path = snapshot / name
    if path.is_symlink():
        content = os.fsencode(os.readlink(path))
    elif path.is_file():
        content = path.read_bytes()
    else:
        raise SystemExit(f'build snapshot missing {name}')
    current[name] = hashlib.sha256(content).hexdigest()

changed = {name: digest for name, digest in current.items() if prior.get(name) != digest}
removed = sorted(set(prior) - set(current))
manifest = {
    'starting_head': 'e9d8ecd9d9d6576402becb8fa97c16e46edd89ae',
    'base_phase9b_delta_sha256': hashlib.sha256(sources[-1].with_name('source-delta.tar.gz').read_bytes()).hexdigest(),
    'build_source': str(snapshot),
    'prior_files': len(prior),
    'current_files': len(current),
    'changed_or_added': changed,
    'removed': removed,
}
data = (json.dumps(manifest, sort_keys=True, indent=2) + '\n').encode()
(out / 'source-delta.json').write_bytes(data)
with (out / 'source-delta.tar.gz').open('wb') as raw_out:
    with gzip.GzipFile(filename='', mode='wb', fileobj=raw_out, mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode='w') as archive:
            info = tarfile.TarInfo('SOURCE-DELTA.json')
            info.size = len(data)
            info.mode = 0o644
            info.mtime = 0
            archive.addfile(info, io.BytesIO(data))
            for name in changed:
                path = snapshot / name
                entry = tarfile.TarInfo(name)
                entry.mode = stat.S_IMODE(path.lstat().st_mode)
                entry.mtime = 0
                if path.is_symlink():
                    entry.type = tarfile.SYMTYPE
                    entry.linkname = os.readlink(path)
                    archive.addfile(entry)
                else:
                    content = path.read_bytes()
                    entry.size = len(content)
                    archive.addfile(entry, io.BytesIO(content))
print(f'{len(changed)} changed or added, {len(removed)} removed, {len(current)} current')
