#!/usr/bin/env bash
# qemu-system-x86_64, or qemu-img when installed as kit-qemu-img.sh, from the
# spike Flatpak on the live session's own desktop. kit.sh copies this off the
# stick and sets KIT_DIR, which is granted explicitly since it lives on a
# removable drive.
#
# The render server gets one Vulkan driver directory with llvmpipe disabled:
# the runtime lists each manifest twice, and a visible llvmpipe made guest
# apps render on the host CPU by default (see FINDINGS.md).
#
# KIT_ISOLATE=1 cuts the session and accessibility buses, for trying the kit
# inside an omabox without reaching the real session.
set -euo pipefail
kit=${KIT_DIR:?run through kit.sh}
command=qemu-system-x86_64
[ "$(basename "$0")" = kit-qemu-img.sh ] && command=qemu-img
isolate=()
[ "${KIT_ISOLATE:-}" = 1 ] && isolate=(--no-session-bus --no-a11y-bus --no-documents-portal)
exec flatpak run --user "${isolate[@]}" \
  --filesystem="$kit" \
  --env=VK_DRIVER_FILES=/usr/lib/x86_64-linux-gnu/GL/vulkan/icd.d \
  --env='VK_LOADER_DRIVERS_DISABLE=*lvp*' \
  --command="$command" com.tryomarchy.TryOmarchy "$@"
