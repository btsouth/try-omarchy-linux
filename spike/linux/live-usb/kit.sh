#!/usr/bin/env bash
# Try Omarchy on Linux, Phase 0: hardware checks from a live USB session.
# Run from the kit directory with bash, since removable drives may drop the
# executable bit. CHECKLIST.md gives the order.
#   bash kit.sh check | install | boot [venus|gl|cpu] | guest-tests | guest-info
#               | sound | idle-cpu | note TEXT | stop | collect
set -euo pipefail
kit=$(cd "$(dirname "$0")" && pwd)
app=com.tryomarchy.TryOmarchy
results=$kit/results
share="$kit/Omarchy Shared"
mkdir -p "$results" "$share"

# The wrappers and SSH key are copied off the stick: exFAT keeps neither the
# executable bit nor private file modes, and ssh refuses a key it cannot see
# as private.
work=${XDG_RUNTIME_DIR:-/tmp}/try-omarchy-kit
mkdir -p "$work"
install -m 700 "$kit/kit-qemu.sh" "$work/kit-qemu.sh"
install -m 700 "$kit/kit-qemu.sh" "$work/kit-qemu-img.sh"
install -m 600 "$kit/spike_key" "$work/spike_key"
export KIT_DIR=$kit SPIKE_DIR=$kit SPIKE_KEY=$work/spike_key

g() { bash "$kit/guest-ssh.sh" -- "$@"; }
gk() { g "bash /tmp/guest-kit.sh $*"; }

wait_guest() {
  for _ in $(seq 1 90); do g true 2>/dev/null && break; sleep 2; done
  g true || { echo "guest did not answer on SSH; see $results/qemu-*.log" >&2; exit 1; }
  for _ in $(seq 1 60); do
    g 'ls /run/user/$(id -u)/hypr 2>/dev/null | grep -q .' && break
    sleep 2
  done
  g 'cat > /tmp/guest-kit.sh' < "$kit/guest-kit.sh"
}

cmd_check() {
  {
    echo "date: $(date -Is)"
    . /etc/os-release && echo "os: $PRETTY_NAME"
    echo "kernel: $(uname -r)"
    echo "desktop: ${XDG_CURRENT_DESKTOP:-?}, session: ${XDG_SESSION_TYPE:-?}"
    echo "cpu:$(grep -m1 'model name' /proc/cpuinfo | cut -d: -f2-)"
    echo "memory: $(free -h | awk '/^Mem:/ {print $2 " total, " $7 " available"}')"
    for d in /sys/class/drm/card*/device; do
      [ -e "$d/vendor" ] || continue
      echo "gpu: $(cat "$d/vendor"):$(cat "$d/device"), driver $(basename "$(readlink "$d/driver")")"
    done | sort -u
    echo "kvm: $([ -r /dev/kvm ] && [ -w /dev/kvm ] && echo usable || echo 'missing or not writable')"
    echo "cpu virtualization flag: $(grep -m1 -owE 'svm|vmx' /proc/cpuinfo || echo none)"
    echo "flatpak: $(flatpak --version 2>/dev/null || echo missing)"
    echo "kit free space: $(df -h --output=avail "$kit" | tail -1)"
  } | tee "$results/host.txt"
}

cmd_install() {
  if ! command -v flatpak >/dev/null; then
    echo "flatpak is missing. On Ubuntu run: sudo apt install -y flatpak" >&2
    exit 1
  fi
  flatpak remote-add --user --if-not-exists flathub https://dl.flathub.org/repo/flathub.flatpakrepo
  flatpak install --user -y --noninteractive flathub \
    org.freedesktop.Platform//25.08 org.freedesktop.Platform.GL.default//25.08
  flatpak install --user -y --noninteractive --reinstall --bundle "$kit/$app.flatpak"
  flatpak run --user --command=qemu-system-x86_64 "$app" --version | head -1 | tee -a "$results/host.txt"
}

cmd_boot() {
  local mode=${1:-venus} args=(--display sdl --share "$share")
  case $mode in
    venus) ;;
    gl) args+=(--no-venus) ;;
    cpu) args+=(--cpu-render) ;;
    *) echo "boot mode is venus, gl or cpu" >&2; exit 2 ;;
  esac
  if pgrep -x qemu-system-x86 >/dev/null; then
    echo "a VM is already running; run: bash kit.sh stop" >&2
    exit 1
  fi
  [ -f "$kit/run/disk.qcow2" ] || args+=(--fresh)
  SPIKE_AUDIO=${SPIKE_AUDIO:-pipewire} QEMU="$work/kit-qemu.sh" QEMU_IMG="$work/kit-qemu-img.sh" \
    setsid bash "$kit/boot.sh" "${args[@]}" >"$results/qemu-$mode.log" 2>&1 </dev/null &
  echo "booting with $mode rendering; the window opens in a few seconds"
  wait_guest
  gk awake
  echo "guest is up"
}

cmd_guest_info() {
  wait_guest
  gk info | tee -a "$results/guest-info.txt"
}

cmd_guest_tests() {
  wait_guest
  gk awake
  echo "installing test tools in the guest (needs network)..."
  g 'sudo pacman -Sy --noconfirm --needed vulkan-tools mesa-utils >/dev/null 2>&1' ||
    echo "pacman failed; continuing with what the guest has"
  gk info | tee "$results/guest-info.txt"
  # Vulkan presentation at a width whose rows are not a multiple of 256 bytes,
  # one whose rows are, and tiled; then vkcube (FP16) and GLES for comparison.
  local c
  for c in "vkgears-1000x700 1000x700 vkgears" "vkgears-1024x768 1024x768 vkgears" \
           "vkgears-tiled tiled vkgears" "vkcube-tiled tiled vkcube --wsi wayland" \
           "es2gears-tiled tiled es2gears_wayland"; do
    set -- $c
    gk case "$@" >"$results/$1.txt" 2>&1 || true
    mv "$share/$1.png" "$results/" 2>/dev/null || true
    echo "$1: $(grep -o 'add(fd [0-9]*, 0, 0, [0-9]*' "$results/$1.txt" | awk '{print "stride " $5}')"
  done
  echo "screenshots and logs are in $results"
}

cmd_idle_cpu() {
  local pid a b hz
  pid=$(pgrep -x qemu-system-x86 | head -1)
  [ -n "$pid" ] || { echo "no VM is running" >&2; exit 1; }
  hz=$(getconf CLK_TCK)
  echo "measuring for 30 s; leave the guest alone"
  a=$(awk '{print $14 + $15}' "/proc/$pid/stat"); sleep 30; b=$(awk '{print $14 + $15}' "/proc/$pid/stat")
  echo "QEMU idle CPU: $(((b - a) * 100 / (hz * 30)))% of one core" | tee -a "$results/idle.txt"
}

cmd_collect() {
  local out
  out=$kit/results-$(date +%Y%m%d-%H%M).tar.gz
  tar -czf "$out" -C "$kit" results
  echo "$out"
}

case ${1:-} in
  check) cmd_check ;;
  install) cmd_install ;;
  boot) shift; cmd_boot "$@" ;;
  guest-tests) cmd_guest_tests ;;
  guest-info) cmd_guest_info ;;
  sound) wait_guest; gk tone ;;
  idle-cpu) cmd_idle_cpu ;;
  note) shift; printf '%s %s\n' "$(date +%T)" "$*" | tee -a "$results/notes.txt" ;;
  stop) bash "$kit/stop.sh" ;;
  collect) cmd_collect ;;
  *) sed -n '5,6p' "$0"; exit 2 ;;
esac
