#!/usr/bin/env bash
# Boot in an omabox SDL window and look for host "not responding" dialogs at
# idle, under an OpenGL load, and under a Venus Vulkan load.
#   staged-gpu-test.sh BOX [boot.sh options...]
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
box=$1; shift
out=${SPIKE_DIR:-/data/try-omarchy-linux-spike}/run
# QEMU=flatpak-qemu.sh brings its own libraries; otherwise use the Arch tree.
[ -n "${QEMU:-}" ] || . "$here/spike-env.sh"
OMABOX_NAME=$box "$here/in-box.sh" "$here/boot.sh" --fresh --display "${SPIKE_DISPLAY:-sdl}" "$@" >"$out/qemu-stderr.log" 2>&1 &
anr() { omabox hyprctl -b "$box" -j clients | jq -r '[.[] | select(.class == "hyprland-dialog")] | length'; }
alive() { pgrep -x qemu-system-x86 >/dev/null && echo alive || echo DEAD; }
g() { "$here/guest-ssh.sh" -- "$@"; }
for i in $(seq 1 60); do g true 2>/dev/null && break; sleep 2; done
for i in $(seq 1 30); do g 'hyprctl -j instances 2>/dev/null | jq -e length' >/dev/null 2>&1 && break; sleep 2; done
g 'sudo pacman -Sy --noconfirm --needed vulkan-tools mesa-utils >/dev/null 2>&1' || true
sleep 60
echo "idle 60s: qemu=$(alive) dialogs=$(anr)"
env='export XDG_RUNTIME_DIR=/run/user/$(id -u); export WAYLAND_DISPLAY=$(hyprctl -j instances | jq -r ".[0].wl_socket"); . /usr/local/lib/try-omarchy/vulkan-env; export VN_PERF VK_LOADER_DISABLE_DYNAMIC_LIBRARY_UNLOADING'
g "$env; timeout 30 es2gears_wayland >/tmp/gears.log 2>&1; tail -1 /tmp/gears.log"
omabox shot -b "$box" -o "$out/stage-gl.png" >/dev/null
echo "after 30s GLES gears: qemu=$(alive) dialogs=$(anr)"
g "$env; timeout 30 vkcube --wsi wayland --gpu_number 0 >/tmp/vk.log 2>&1 & sleep 12; echo" >/dev/null
omabox shot -b "$box" -o "$out/stage-vk.png" >/dev/null
sleep 20
echo "after 30s vkcube: qemu=$(alive) dialogs=$(anr)"
g 'head -2 /tmp/vk.log' 2>/dev/null
grep -v '^$' "$out/qemu-stderr.log" | sort | uniq -c | sort -rn | head -5
