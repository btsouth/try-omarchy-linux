#!/usr/bin/env python3
"""Pin the GTK helper's Go module downloads for an offline Flatpak build.

Run after changing linux-ui/go.mod or go.sum. Go verifies the cached modules
against go.sum before their archive hashes are recorded here.
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

here = Path(__file__).resolve().parent
module_dir = sys.argv[1] if len(sys.argv) > 1 else "linux-ui"
if module_dir not in ("linux-ui", "app"):
    raise SystemExit("usage: generate-go-sources.py [linux-ui|app]")
output = subprocess.check_output(
    ["go", "mod", "download", "-json", "all"], cwd=here.parent.parent / module_dir, text=True
)
decoder = json.JSONDecoder()
sources = []
while output.strip():
    module, end = decoder.raw_decode(output.lstrip())
    output = output.lstrip()[end:]
    if "Error" in module:
        raise SystemExit(module["Error"])
    path = "".join("!" + c.lower() if c.isupper() else c for c in module["Path"])
    version = module["Version"]
    for field, suffix in [("Info", "info"), ("GoMod", "mod"), ("Zip", "zip")]:
        sources.append({
            "type": "file",
            "url": f"https://proxy.golang.org/{path}/@v/{version}.{suffix}",
            "sha256": hashlib.sha256(Path(module[field]).read_bytes()).hexdigest(),
            "dest": f"goproxy/{path}/@v",
        })
(here / ("app-go-sources.json" if module_dir == "app" else "go-sources.json")).write_text(json.dumps(sources, indent=2) + "\n")
