# Linux desktop integration, 2026-09-26

Uncommitted local candidate on `linux-core`, based on `e9d8ecd`. No publication.
This continues the earlier setup-window evidence. Implementation and isolated
acceptance are complete within the boundaries recorded below.

## Implemented

- GTK first-run storage and account choices, saved portal folder grants,
  cancellation/resume, close confirmation and settings for the next launch.
- Guest visibility leases expire after 20 seconds. The user's stay-awake
  preference remains independent. Known idle-service versions are repaired
  during compatibility migration; customized versions are preserved.
- Wayland data-control and X11 clipboard adapters reuse the existing bounded
  text, PNG, and streaming-file protocol. Portal file selections are resolved
  through the FileTransfer portal. Explicit SDL drops use the file transport
  even when the desktop denies background clipboard access.
- Portal-selected shared folders, tray settings/shutdown, PipeWire routes,
  microphone disable, and host scale through SDL backing pixels and guest EDID.
- Host keyboard environment detection and explicit layout/scale overrides.
  Desktop keyboard-group changes are not monitored.
- Private build/check/smoke scripts and a disabled CI template.

## Verified checks

| Check | Result |
| --- | --- |
| GTK first run and portal data location on Intel | Passed, including resume |
| Visible guest for over six minutes | Stayed unlocked; stay-awake preference false |
| Guest agent stopped | Lease expired and normal idle behavior returned |
| Close, keep running, shutdown | Passed on the OpenGL fallback; shutdown about 3 seconds |
| Wayland Unicode text, PNG and files | All six directions passed |
| Selected shared folder | Guest writes reached only the selected folder |
| Linux tests with race detector and vet | Passed |
| Windows vet, cross-build and test compilation | Passed; native Windows tests not run |
| Guest contracts, including legacy and Linux scale policies | Passed |
| New guest image | Built with patches 0096 through 0100, compatibility 37 |
| GNOME 46 / Ubuntu 24.04 disposable VM | Fresh image booted; shortcut-inhibition prompt worked |
| Private PipeWire playback and microphone | 440 Hz signal transferred both ways |
| Audio output device removal | Followed the new default and recovered to the dummy sink |
| KDE Wayland and Xfce X11 clipboard | All six text, PNG and file directions passed |
| Microphone disabled on X11 | QEMU exposed an output stream only |
| Small-window fit on final bundle | Passed in X11 and NVIDIA omabox at 1280x800 |

The audio tests used only the disposable desktop VM's dummy sink and monitor
source. Output peak was 768 at the guest's default volume, with a 440 Hz power
ratio of 1.0. Input peak was 11999. Switching to a new dummy sink and removing
it produced peaks of 769 and 768 respectively. No physical audio was used.

## Graphics finding

On the Intel devbox's 6.8.0-139 kernel, Venus froze QEMU during window resize
with `kvm run failed: Bad address`. CPU presentation did not prevent it.
Removing Venus/blob buffers while retaining hardware OpenGL passed the same
resize and close flow. Automatic Venus now requires kernel 6.16 or newer and
requests guest PAT support. `-venus on` remains available for tested backports.
The threshold is conservative; this is not proof that every older kernel fails.
See the [Mesa Venus requirements](https://docs.mesa3d.org/drivers/venus.html).

## Artifacts and boundaries

Devbox source: `/workspace/try-omarchy-linux/phase3-src`.
Build state and bundle: `/workspace/try-omarchy-linux/phase3-flatpak`.
Guest artifacts: `/workspace/try-omarchy-linux/phase6-guest`.
Desktop VM and logs: `/workspace/try-omarchy-linux/desktop-matrix`.
Local evidence: `/data/try-omarchy-linux-spike/phase3-flow`, `phase4`, `phase5`,
and `phase6-guest`.

Guest SHA256SUMS digest:
`9bf24910f99030944abfaf552af237df35a7f93d6df0b989af2373acd4cca8ea`.

The local guest downloads were checked against those sums. Guest kernel is
7.2.7-arch1-1, compatibility 37, builder commit `d5a57b1`.

Final bundle: `/data/try-omarchy-linux-spike/phase5/com.tryomarchy.TryOmarchy.flatpak`.
SHA256: `beae8a169e485363f02546e7e2179917a2c4a0d72c83ee3ce59653eaf75f4590`.
Installed OSTree commit:
`85aa3ab33c9c5822fb0dc52a0428bf8081a75f07ea8b0060d22c66b8a5051d69`.

The final bundle adds a QEMU fix to maximize when a requested window size would
exceed the usable desktop. It was checked on Intel (including guest scale 2),
NVIDIA in an omabox, and Xfce X11. GNOME and KDE matrix checks used the preceding
bundle with the same launcher and GTK helper, before this final QEMU sizing fix.
No exact-final-bundle GNOME/KDE rerun is claimed.

GNOME automatic clipboard synchronization is unsupported. Settings explains
this; shared folders remain available. Physical laptop, real audio, suspend,
and real multi-monitor acceptance remain deferred by request.

## Additional acceptance

KDE Plasma 5.27.12 on Wayland passed all six clipboard directions with the
rebuilt guest. The tray menu opened Settings; invalid memory was rejected,
then 4096 MiB was saved. Dismissing shutdown confirmation left QEMU running.
Confirming shutdown stopped the guest in two seconds (23:54:27 to 23:54:29 UTC).
The settings action buttons now remain visible while the form scrolls.

The Intel compositor scale changed from 1 to 1.5 to 2 and back. Guest modes
were 1916x1076 at scale 1, 1914x1074 at 1.5, and 1912x1072 at 2. SDL2-compat
required `SDL_VIDEO_WAYLAND_SCALE_TO_DISPLAY=0` to expose the fractional scale.
The normal launcher environment, without a test-only override, passed 1.5x.
These are simulated output-scale checks, not physical monitor acceptance.

The existing Intel disk migrated to compatibility 37 and retained its sentinel
file and saved shared-folder grant. The installed bundle also booted a fresh
NVIDIA guest in an omabox and powered it off cleanly. The first screenshot was
taken too early, so the smoke script now waits for the guest shell's IPC reply
before capture. The second screenshot shows the desktop and bar.

Xfce X11 passed all six clipboard directions after repairing the disposable
VM's mixed desktop-session configuration. The final bundle fits within its
1280x800 desktop with window controls visible. Microphone-off left only QEMU's
output stream in PipeWire. The harness failures were not product failures.

## Remaining acceptance and cleanup

A real external-file drop with sandbox permission acquisition and a portal
FileTransfer grant roundtrip still need live acceptance. Unit coverage and the
successful shared-file clipboard roundtrips do not substitute for those checks.
GNOME automatic clipboard and live host keyboard-group tracking remain unsupported.
Physical laptop, real audio, suspend and real monitor checks remain deferred.
The private CI template is not enabled, and native Windows tests were not run.

Evidence includes `phase5/matrix-evidence/{kde,x11}-clipboard.log`,
`phase5/final-checks`, `phase5/local-smoke-final/desktop.png`,
`phase5/local-fit.png`, and `phase5/xfce-desktop.png` under the local evidence root.
The test guests were powered off cleanly. Task desktop sessions and loopback
release servers were stopped; test disks, logs and artifacts were retained.
The main checkout remains untouched. Product changes remain uncommitted; guest
patch-builder commits are local only. Nothing was pushed or published.
