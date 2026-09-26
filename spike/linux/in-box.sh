#!/usr/bin/env bash
# Run a host command (QEMU needs /dev/kvm, which a box lacks) with its window
# and session bus inside the named omabox, never on the real desktop.
set -euo pipefail
box=${OMABOX_NAME:?set OMABOX_NAME to the box name}
run=/run/user/$(id -u)/omabox/$box/run
export WAYLAND_DISPLAY=$run/wayland-1
# The box bus lives in its private /tmp; a dead address keeps QEMU off the real one.
export DBUS_SESSION_BUS_ADDRESS=unix:path=/nonexistent/omabox-spike-bus NO_AT_BRIDGE=1
unset DISPLAY WAYLAND_SOCKET HYPRLAND_INSTANCE_SIGNATURE
export SDL_VIDEODRIVER=wayland GDK_BACKEND=wayland
exec "$@"
