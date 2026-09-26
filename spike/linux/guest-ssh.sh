#!/usr/bin/env bash
# Run a command in the spike guest as the instant-trial user over the
# forwarded SSH port. Usage: guest-ssh.sh [-p PORT] -- command...
set -euo pipefail
spike=${SPIKE_DIR:-/data/try-omarchy-linux-spike}
port=42222
if [ "${1:-}" = -p ]; then port=$2; shift 2; fi
[ "${1:-}" = -- ] && shift
exec ssh -q -i "${SPIKE_KEY:-$spike/spike_key}" -p "$port" \
  -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
  -o ConnectTimeout=5 -o BatchMode=yes omarchy@127.0.0.1 "$@"
