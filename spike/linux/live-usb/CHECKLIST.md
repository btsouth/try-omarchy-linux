# Live USB hardware check

This covers what the Omarchy/NVIDIA desktop could not: an AMD GPU, GNOME,
and a normal distro. Nothing is installed on the laptop. The live session runs
from one stick, and this kit (scripts, the spike Flatpak, the guest image)
runs from a second stick or external drive.

## Before you go

1. Write a Fedora Workstation live ISO to one stick. Fedora ships Flatpak and
   a current GNOME. Ubuntu works too, but needs `sudo apt install flatpak`.
2. On the desktop, format the second stick as exFAT or ext4 (16 GB free at
   least, FAT32 cannot hold the rootfs) and stage the kit:

   ```
   spike/linux/live-usb/make-kit.sh /run/media/$USER/STICK/try-omarchy-kit
   ```

   It copies about 6 GB and checks every file against the release checksums.

## On the laptop

1. In the firmware settings, make sure SVM (AMD virtualization) is on. It
   already is if Windows runs Try Omarchy. Boot the live stick, choose "Try",
   connect to Wi-Fi, and plug in the kit stick.
2. Open a terminal and go to the kit: `cd /run/media/liveuser/*/try-omarchy-kit`
3. `bash kit.sh check` records the machine. Check that the output says
   `kvm: usable`.
4. `bash kit.sh install` adds Flathub for this session and installs the
   runtime (about 700 MB into RAM) and the spike Flatpak.
5. `bash kit.sh boot` opens the Try Omarchy window with GPU rendering, OpenGL
   and Vulkan. The desktop should appear within a minute.
6. `bash kit.sh guest-tests` runs the rendering checks inside the guest and
   saves guest screenshots to `results/`. Watch the window while it runs:
   gears and a cube should draw cleanly, without diagonal streaks.

Then go through the checks below. Record each answer with
`bash kit.sh note "..."` so it ends up in `results/notes.txt`.

## Manual checks

1. Sharpness at 100%: is text in the window as sharp as the host's own?
2. Super key: click into the window and press Super+Space. Did the host or
   the guest react? Press Ctrl+Alt+G to grab input and try again. GNOME
   should ask once whether to let the app inhibit shortcuts. Note the
   wording, allow it, and check that Super+Space now opens the guest's menu.
   Ctrl+Alt+G releases the grab.
3. Resize: drag a window edge. The guest desktop should re-lay out to the new
   size, not stretch. `bash kit.sh guest-info` records the guest's resolution.
4. Scaling: in Settings > Displays pick 125% or 150% if offered, else 200%.
   Is the window still sharp? Run `bash kit.sh guest-info` again. Put the
   scale back afterwards.
5. Close: click the window's close button. With this spike build nothing
   should happen (the product will ask to shut down). Note what you saw.
6. Sound: `bash kit.sh sound` plays a two-second tone in the guest. Did it
   come out of the laptop speakers?
7. Idle cost: leave the guest alone and run `bash kit.sh idle-cpu`.
8. Anything odd: freezes, a "not responding" dialog, a black window, the
   cursor misbehaving.

## Optional second pass

`bash kit.sh stop`, then `bash kit.sh boot gl` (OpenGL only) and
`bash kit.sh guest-tests` again, to compare against Vulkan.

## Finish

1. `bash kit.sh stop`
2. `bash kit.sh collect` writes a `results-*.tar.gz` onto the kit stick.
3. Shut down the live session. Nothing on the laptop's disk changed.

Bring the stick back, or copy the tarball somewhere I can read it.
