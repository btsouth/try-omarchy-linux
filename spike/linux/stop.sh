#!/usr/bin/env bash
# Stop the spike VM through QMP quit, then wait for the process to exit.
run=${SPIKE_DIR:-/data/try-omarchy-linux-spike}/run
python3 - "$run/qmp.sock" <<'PY' 2>/dev/null || true
import json, socket, sys
s = socket.socket(socket.AF_UNIX); s.settimeout(5); s.connect(sys.argv[1]); f = s.makefile('rw')
f.readline()
for cmd in ({"execute": "qmp_capabilities"}, {"execute": "quit"}):
    f.write(json.dumps(cmd) + "\n"); f.flush(); f.readline()
PY
for _ in $(seq 1 40); do pgrep -x qemu-system-x86 >/dev/null || exit 0; sleep 0.25; done
echo "QEMU did not exit after QMP quit" >&2; exit 1
