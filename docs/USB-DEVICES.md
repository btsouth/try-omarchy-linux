# USB devices

USB passthrough is experimental and depends on the device's Windows driver.
Try Omarchy does not replace drivers.

## USB selection before launch

Settings > Devices > **USB device for next start...** remembers one device at
its current USB port. Saving the choice asks before granting startup access.
Windows applications lose access while Omarchy owns the device. Eject mounted
storage before switching it. **Don't attach** disables startup attachment while
retaining the saved choice.

The runtime lists devices without starting a guest or taking a device away from
Windows. A missing choice stays visible. Moving it to another port requires a
new selection. Missing, busy or unsupported devices do not prevent Omarchy from
starting. The launcher tries once per VM boot and does not reclaim an unplugged
and reconnected device automatically. Live Attach and Release remain available
from the tray's USB devices menu. No driver is installed by this flow.

## Linux

The Linux launcher offers the same startup choice in Settings > Devices and
live Attach/Release from the tray's **USB devices** menu. Devices are listed
from `/sys/bus/usb/devices` without opening device nodes. Hubs and root hubs are
excluded. Identity is the USB bus, physical port path and vendor/product IDs;
a changed enumeration address is resolved at attachment time. The runtime uses
an xHCI controller and QMP `device_add`/`device_del` with QEMU's libusb `usb-host`.
It never automatically reclaims a disconnected device.

The Flatpak requests `--device=usb`, supported by Flatpak 1.16 and newer,
rather than `--device=all`. Older hosts ignore this permission; the VM still
starts and reports an unavailable USB attachment. `--usb=` filters govern the
USB portal's enumeration, not direct libusb access. QEMU opens device paths
rather than portal-provided descriptors, so those filters cannot narrow this
permission. The launcher only claims a device explicitly selected by the user.

The host user also needs read and write permission on the device node under
`/dev/bus/usb`. Host udev `uaccess` rules cover many devices, but not every
device. If access is denied, the launcher explains the missing user or Flatpak
access and continues running. It does not install drivers, change host
permissions or request root access. Busy devices may require closing host
applications. Eject mounted storage before attaching it to Omarchy.
