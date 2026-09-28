#!/usr/bin/env bash
# Exercise the installed bundle in an invisible omabox with host audio and
# session services denied. Evidence and a disposable VM stay in OUTPUT_DIR.
set -euo pipefail
out=${1:?usage: smoke-flatpak.sh OUTPUT_DIR RELEASE_URL SHA256SUMS_DIGEST}
release=${2:?missing release URL}
sums=${3:?missing SHA256SUMS digest}
[[ $sums =~ ^[0-9a-fA-F]{64}$ ]] || { echo 'Invalid digest' >&2; exit 2; }
mkdir -p "$out"
out=$(realpath "$out")
[[ ! -e $out/vm && ! -e $out/key ]] || { echo 'Use a fresh output directory' >&2; exit 2; }
for tool in omabox flatpak ssh ssh-keygen python3; do command -v "$tool" >/dev/null; done
[[ -r /dev/kvm && -w /dev/kvm ]] || { echo 'KVM access required' >&2; exit 1; }
port=$(python3 - <<'PY'
import socket
with socket.socket() as s:
 s.bind(('127.0.0.1',0)); print(s.getsockname()[1])
PY
)
box=try-omarchy-smoke-$$
pid=
cleanup() {
 if [[ -n $pid ]] && kill -0 "$pid" 2>/dev/null; then
  kill -INT "$pid" 2>/dev/null || true
  for ((i=0;i<30;i++)); do kill -0 "$pid" 2>/dev/null || break; sleep 1; done
  kill -TERM "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
 fi
 omabox down -b "$box" >/dev/null 2>&1 || true
}
trap cleanup EXIT
ssh-keygen -q -t ed25519 -N '' -f "$out/key"
omabox up -b "$box" --no-shell --net isolated
run=/run/user/$(id -u)/omabox/$box/run
flatpak run --user --no-session-bus --no-a11y-bus --no-documents-portal \
 --nosocket=wayland --nosocket=x11 --nosocket=fallback-x11 \
 --nosocket=pulseaudio --nofilesystem=xdg-run/pipewire-0 \
 --filesystem="xdg-run/omabox/$box/run" --filesystem="$out" \
 --env=BOX_WAYLAND="$run/wayland-1" --unset-env=DISPLAY \
 --command=sh com.tryomarchy.TryOmarchy \
 -c 'export WAYLAND_DISPLAY=$BOX_WAYLAND; exec try-omarchy "$@"' try-omarchy \
 -no-gui -dir "$out/vm" -release "$release" -sums-sha256 "$sums" \
 -instant -memory 3072 -cpus 2 -ssh "$port" -ssh-key "$out/key.pub" \
 >"$out/launcher.log" 2>&1 < /dev/null &
pid=$!
ssh_args=(-i "$out/key" -p "$port" -o BatchMode=yes -o ConnectTimeout=2
 -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="$out/known_hosts")
ready=false
for ((i=0;i<300;i++)); do
 kill -0 "$pid" 2>/dev/null || { cat "$out/launcher.log"; exit 1; }
 if ssh "${ssh_args[@]}" omarchy@127.0.0.1 \
  'test -S /run/user/1000/wayland-1 && systemctl is-active try-omarchy-agent && XDG_RUNTIME_DIR=/run/user/1000 DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus OMARCHY_PATH=/usr/share/omarchy omarchy-shell shell ping' \
  >"$out/guest-ready.log" 2>"$out/ssh.log"; then ready=true; break; fi
 sleep 2
done
[[ $ready == true ]] || { echo 'Guest did not become ready' >&2; exit 1; }
ssh "${ssh_args[@]}" omarchy@127.0.0.1 \
 'uname -a; cat /usr/share/try-omarchy/compat-version; cat /proc/cmdline' >"$out/guest.log"
# Allow the first rendered frame after the shell answers IPC.
sleep 3
omabox shot -b "$box" -o "$out/desktop.png"
ssh "${ssh_args[@]}" omarchy@127.0.0.1 'sudo systemctl poweroff' || true
for ((i=0;i<45;i++)); do kill -0 "$pid" 2>/dev/null || break; sleep 1; done
if kill -0 "$pid" 2>/dev/null; then echo 'Guest did not shut down' >&2; exit 1; fi
wait "$pid"
pid=
flatpak info --user com.tryomarchy.TryOmarchy >"$out/flatpak.txt"
printf 'Packaged KVM boot and shutdown passed. Inspect desktop.png for visual acceptance.\n'
