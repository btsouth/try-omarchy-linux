# Runs inside the guest, pushed there by kit.sh over SSH.
#   guest-kit.sh awake | info | tone | case NAME SIZE COMMAND...
# SIZE is WxH for a floating window or "tiled".
export XDG_RUNTIME_DIR=/run/user/$(id -u)
export WAYLAND_DISPLAY=$(ls "$XDG_RUNTIME_DIR" | grep -m1 '^wayland-[0-9]$')
export HYPRLAND_INSTANCE_SIGNATURE=$(ls "$XDG_RUNTIME_DIR/hypr" | head -1)
. /usr/local/lib/try-omarchy/vulkan-env
export VN_PERF VK_LOADER_DISABLE_DYNAMIC_LIBRARY_UNLOADING
# The host folder's 9p mount; the home link is named after the host folder.
share=$(findmnt -n -o TARGET -t 9p | head -1)

# Process names are cut to 15 characters; -f would match this script's own
# arguments.
stop_clients() { pkill -x vkgears; pkill -x vkcube; pkill -x es2gears_waylan; sleep 0.5; }

case $1 in
  awake)
    omarchy-toggle-idle stay-awake >/dev/null
    # First boot opens the shared folder; close it so test windows tile alone.
    pkill -x nautilus
    ;;
  info)
    echo "== GL"; eglinfo -B 2>/dev/null | grep -m6 -iE 'renderer|version'
    echo "== Vulkan"; vulkaninfo --summary 2>/dev/null | grep -E 'deviceName|driverID|driverInfo'
    echo "== monitors"; hyprctl monitors | grep -E '^Monitor|@|scale:'
    ;;
  tone)
    python3 -c '
import math, struct, wave
w = wave.open("/tmp/tone.wav", "wb")
w.setnchannels(1); w.setsampwidth(2); w.setframerate(48000)
w.writeframes(b"".join(struct.pack("<h", int(8000 * math.sin(2 * math.pi * 440 * i / 48000))) for i in range(96000)))'
    pw-play /tmp/tone.wav
    ;;
  case)
    name=$2 size=$3; shift 3
    stop_clients
    VN_DEBUG=wsi MESA_LOG_LEVEL=debug WAYLAND_DEBUG=1 timeout 20 "$@" >"/tmp/$name.log" 2>&1 &
    sleep 3
    if [ "$size" != tiled ]; then
      hyprctl dispatch 'hl.dsp.window.float({ action = "enable" })' >/dev/null
      hyprctl dispatch "hl.dsp.window.resize({ x = ${size%x*}, y = ${size#*x} })" >/dev/null
    fi
    sleep 5
    grim "$share/$name.png"
    hyprctl -j activewindow | jq -c '{class, size}'
    grep -m1 'Selected GPU' "/tmp/$name.log"
    grep -m1 'prime_blit' "/tmp/$name.log"
    grep 'buffer_params.*add' "/tmp/$name.log" | tail -1
    grep 'create_immed' "/tmp/$name.log" | tail -1
    stop_clients
    ;;
esac
true
