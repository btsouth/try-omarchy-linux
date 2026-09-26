#!/usr/bin/env bash
# qemu-system-x86_64 from the spike Flatpak, drawing into an omabox only.
# Flatpak's own display sockets are removed so the window cannot fall back to
# the real session's wayland-0; the box's socket is bound in explicitly and
# named inside the sandbox, since --nosocket=wayland clears WAYLAND_DISPLAY. The
# session bus is cut too: SDL would otherwise inhibit the real screensaver
# through the portal.
#
# Venus sees whatever Vulkan drivers the render server's loader finds. The
# runtime lists each manifest in two directories, so every host GPU showed up
# twice, and its llvmpipe showed up too: Mesa's device-select layer in the
# guest then preferred Venus-on-llvmpipe (CPU rendering on the host) for
# ordinary Vulkan apps. One directory, llvmpipe disabled.
#   OMABOX_NAME=box QEMU=spike/linux/flatpak-qemu.sh spike/linux/boot.sh ...
set -euo pipefail
box=${OMABOX_NAME:?set OMABOX_NAME to the box name}
run=/run/user/$(id -u)/omabox/$box/run
exec flatpak run --user \
  --no-session-bus --no-a11y-bus --no-documents-portal \
  --nosocket=wayland --nosocket=x11 --nosocket=fallback-x11 \
  --filesystem="xdg-run/omabox/$box/run" \
  --env=BOX_WAYLAND="$run/wayland-1" --env=SDL_VIDEODRIVER=wayland \
  --unset-env=DISPLAY --unset-env=RENDER_SERVER_EXEC_PATH --unset-env=QEMU_MODULE_DIR \
  --env=VK_DRIVER_FILES=/usr/lib/x86_64-linux-gnu/GL/vulkan/icd.d \
  --env='VK_LOADER_DRIVERS_DISABLE=*lvp*' \
  --command=sh com.tryomarchy.TryOmarchy \
  -c 'export WAYLAND_DISPLAY=$BOX_WAYLAND; exec qemu-system-x86_64 "$@"' qemu "$@"
