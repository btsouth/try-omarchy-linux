#!/usr/bin/env python3
"""Write the Flathub submission for one app release.

    make-flathub-submission.py TAG COMMIT OUTPUT_DIR

The manifest here builds from this checkout. Flathub builds from a pinned git
tag instead, with the patches and offline Go sources beside its manifest.
OUTPUT_DIR receives that manifest, the files it references and flathub.json,
ready to copy into the submission pull request.
"""
from pathlib import Path
import re
import shutil
import sys

REPO_URL = "https://github.com/btsouth/try-omarchy-linux.git"
APP = "com.tryomarchy.TryOmarchy"


def main() -> None:
    if len(sys.argv) != 4:
        raise SystemExit(__doc__.strip().splitlines()[2].strip())
    tag, commit, out = sys.argv[1], sys.argv[2], Path(sys.argv[3])
    if not re.fullmatch(r"linux-app-v\d+\.\d+\.\d+(-preview\.\d+)?", tag):
        raise SystemExit(f"not an app release tag: {tag}")
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise SystemExit(f"not a full commit hash: {commit}")
    here = Path(__file__).resolve().parent
    manifest = (here / f"{APP}.yml").read_text()
    out.mkdir(parents=True, exist_ok=True)
    if any(out.iterdir()):
        raise SystemExit(f"{out} must be empty")

    git_source = (
        "      - type: git\n"
        f"        url: {REPO_URL}\n"
        f"        tag: {tag}\n"
        f"        commit: {commit}\n"
    )

    def replace(old: str, new: str) -> None:
        nonlocal manifest
        if manifest.count(old) != 1:
            raise SystemExit(f"manifest changed; update this script near: {old.strip()[:60]}")
        manifest = manifest.replace(old, new)

    # Both Go programs build from the tagged checkout.
    for part in ("linux-ui", "app"):
        replace(
            f"      - type: dir\n        path: ../../{part}\n        dest: {part}\n",
            git_source,
        )
    # The desktop file and metainfo come from the same checkout.
    for name in (f"{APP}.desktop", f"{APP}.metainfo.xml"):
        replace(f"      - install -Dm644 {name} ", f"      - install -Dm644 runtime-build/linux/{name} ")
        replace(f"      - type: file\n        path: {name}\n", "")
    replace("# Build with build-flatpak.sh beside this file.\n",
            f"# Generated for Flathub from {tag} ({commit}) by\n"
            "# runtime-build/linux/make-flathub-submission.py. Edit the source manifest.\n")

    # Local files the manifest still names: patches, Go sources, C helper.
    for match in re.finditer(r"^\s+path: (\S+)$", manifest, re.M):
        source = (here / match.group(1)).resolve()
        target = Path("patches/qemu") / source.name if source.suffix == ".patch" else Path(source.name)
        (out / target).parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, out / target)
        manifest = manifest.replace(f"path: {match.group(1)}\n", f"path: {target.as_posix()}\n", 1)
    for name in ("go-sources.json", "app-go-sources.json"):
        shutil.copyfile(here / name, out / name)

    (out / f"{APP}.yml").write_text(manifest)
    (out / "flathub.json").write_text('{\n  "only-arches": ["x86_64"]\n}\n')
    print(f"Flathub submission for {tag} written to {out}")


if __name__ == "__main__":
    main()
