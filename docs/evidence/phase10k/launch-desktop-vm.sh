#!/usr/bin/env bash
set -euo pipefail
root=/workspace/try-omarchy-linux/phase10k-retained
exec qemu-system-x86_64 \
  -machine q35,accel=kvm -cpu host -smp 4 -m 8192 \
  -drive file="$root/desktop-overlay.qcow2",format=qcow2,if=virtio \
  -netdev user,id=n0,hostfwd=tcp:127.0.0.1:22237-:22 \
  -device virtio-net-pci,netdev=n0 \
  -device virtio-vga -device qemu-xhci -device usb-tablet \
  -display none -vnc 127.0.0.1:11 \
  -virtfs local,path="$root/artifacts",mount_tag=artifacts,security_model=none,readonly=on \
  -serial file:"$root/serial.log" \
  -daemonize -pidfile "$root/qemu.pid"
