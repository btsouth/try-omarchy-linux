#!/usr/bin/env bash
# Phase 0 spike: boot the released x86_64 guest under KVM on a Linux host with
# the same devices the Windows launcher uses (app/qemu.go), swapping WHPX for
# KVM and choosing the display backend. Not a product launcher.
#
#   boot.sh [--display egl-headless|sdl|gtk|dbus] [--cpu-render] [--no-venus] [--no-blob]
#           [--vnc N] [--ssh PORT] [--mem MIB] [--smp N] [--fresh] [--share DIR]
#
# Environment:
#   SPIKE_DIR        work dir with guest/ and spike_key (default /data/try-omarchy-linux-spike)
#   QEMU             qemu-system-x86_64 to run (default: from PATH)
#   QEMU_IMG         qemu-img to run (default: from PATH)
#   QEMU_MODULE_DIR, LD_LIBRARY_PATH  passed through for a private module tree
set -euo pipefail

spike=${SPIKE_DIR:-/data/try-omarchy-linux-spike}
guest=$spike/guest
qemu=${QEMU:-qemu-system-x86_64}
qemu_img=${QEMU_IMG:-qemu-img}
display=egl-headless
gpu=1
venus=on
blob=on
vnc=
ssh_port=42222
mem=4096
smp=4
fresh=0
share=

while [ $# -gt 0 ]; do
  case $1 in
    --display) display=$2; shift 2 ;;
    --cpu-render) gpu=0; shift ;;
    --no-venus) venus=off; shift ;;
    --no-blob) blob=off; venus=off; shift ;;
    --vnc) vnc=$2; shift 2 ;;
    --ssh) ssh_port=$2; shift 2 ;;
    --mem) mem=$2; shift 2 ;;
    --smp) smp=$2; shift 2 ;;
    --fresh) fresh=1; shift ;;
    --share) share=$2; shift 2 ;;
    *) echo "unknown option $1" >&2; exit 2 ;;
  esac
done

run=$spike/run
mkdir -p "$run"
disk=$run/disk.qcow2
# A qcow2 overlay keeps the factory rootfs pristine; 24 GiB matches the
# spec's expandedSizeMiB.
if [ $fresh = 1 ] || [ ! -e "$disk" ]; then
  rm -f "$disk"
  "$qemu_img" create -q -f qcow2 -b "$guest/rootfs.ext4" -F raw "$disk" 24G
fi

# python3 rather than jq: live sessions ship the former.
base=$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["runtime"]["kernelCommandLine"])' "$guest/build-spec.json")
cmdline=${base/console=tty0 /}
cmdline=${cmdline/console=hvc0/console=ttyS0}
cmdline+=" vt.global_cursor_default=0 tryomarchy.instant=1"
cmdline+=" tryomarchy.sshd=1 tryomarchy.sshkey=$(base64 -w0 < "$spike/spike_key.pub")"
cmdline+=" video=1920x1080"
if [ -n "$share" ]; then
  cmdline+=" tryomarchy.sharename=$(printf %s "$(basename "$share")" | base64 -w0)"
fi

args=(-machine q35,accel=kvm -smp "$smp" -m "${mem}M" -name "Try Omarchy")
if [ $gpu = 1 ]; then
  hostmem=$((4 << 30))
  if [ $blob = on ]; then
    args+=(-cpu host -device "virtio-vga-gl,blob=on,hostmem=$hostmem,venus=$venus")
  else
    args+=(-cpu host -device virtio-vga-gl)
  fi
  cmdline+=" tryomarchy.render=gpu"
  case $display in
    egl-headless) args+=(-display egl-headless,rendernode=/dev/dri/renderD128) ;;
    sdl) args+=(-display sdl,gl=on,show-cursor=off,window-close=off) ;;
    gtk) args+=(-display gtk,gl=on,show-cursor=off,window-close=off,zoom-to-fit=on) ;;
    dbus) args+=(-display dbus,gl=on) ;;
    *) echo "unknown display $display" >&2; exit 2 ;;
  esac
else
  args+=(-cpu host -vga none -device virtio-gpu-pci,id=gpu0)
  cmdline+=" tryomarchy.render=cpu"
  case $display in
    egl-headless) args+=(-display none) ;;
    sdl) args+=(-display sdl,gl=off,show-cursor=off,window-close=off) ;;
    gtk) args+=(-display gtk,gl=off,show-cursor=off,window-close=off,zoom-to-fit=on) ;;
    dbus) args+=(-display dbus) ;;
  esac
fi
[ -n "$vnc" ] && args+=(-vnc "127.0.0.1:$vnc")

args+=(
  -drive "file=$disk,format=qcow2,if=virtio"
  -kernel "$guest/vmlinuz-linux" -initrd "$guest/initramfs-linux.img"
  -append "$cmdline"
  -device virtio-keyboard-pci -device virtio-tablet-pci
  -device virtio-net-pci,netdev=n0 -netdev "user,id=n0,hostfwd=tcp:127.0.0.1:$ssh_port-:22"
  -device virtio-rng-pci
  -device virtio-serial-pci,id=virtioserial0
  -device qemu-xhci,id=usb0
  -global ICH9-LPC.disable_s3=1 -global ICH9-LPC.disable_s4=1
  -audiodev "${SPIKE_AUDIO:-none},id=snd" -device virtio-sound-pci,audiodev=snd
  -device virtio-balloon-pci,free-page-reporting=on
  -monitor none -parallel none
  -qmp "unix:$run/qmp.sock,server=on,wait=off"
  -serial "file:$run/serial.log"
  -D "$run/qemu.log"
)

if [ -n "$share" ]; then
  args+=(-virtfs "local,path=${share//,/,,},mount_tag=hostshare,security_model=none")
fi

echo "$qemu ${args[*]}" > "$run/cmdline.txt"
exec "$qemu" "${args[@]}"
