# Private Linux release continuation: GNOME Wayland clipboard

Status on 2026-09-27: consented automatic clipboard sync passed on the
installed GNOME 46 Wayland Flatpak with the private r39 guest. This closes the
specific GNOME product decision, not Phase 9, Phase 10, Phase 11 or publication.
Everything in this report remained local. No commit, push, PR, issue, release,
distribution endpoint or real-desktop operation occurred.

## Exact source, bundle and guest

- Worktree: `/home/bts/Projects/try-omarchy-linux-core`, branch `linux-core`,
  unchanged starting HEAD `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`.
  The inherited uncommitted work remains in place. The exact devbox build input
  is retained at `/workspace/try-omarchy-linux/phase10d-gnome-src`, with a local
  copy at `/home/bts/.cache/try-omarchy-phase10d-source`. Its reconstructable
  delta relative to the Phase 9B source manifest is
  [`phase10c/source-delta.tar.gz`](phase10c/source-delta.tar.gz), SHA256
  `3f4d325a01f08a6082c5daa66faf794eaab318f095e09dcf7c65c8da33f3e24f`.
  The manifest lists 717 current source files, 23 changes/additions and zero
  removals. Its Phase 9B base delta SHA256 is
  `50ea908107dd95483e3b945cdaf7f490efde622d18ab5fa9a667e5db5ef4cdad`.
  The archive contents were checked against every changed-file hash. App,
  `linux-ui` and runtime-build files in the current worktree match the frozen
  build snapshot. Later evidence and ledger edits are outside that snapshot.
- Bundle: [`phase10c/phase10d-gnome.flatpak`](phase10c/phase10d-gnome.flatpak),
  SHA256 `25c5a7fd3f16d62eb399e45c31aa608988d299d48ce6c19ad06b0b99cbd9cef5`.
  The devbox build log is [`phase10c/flatpak-build.log`](phase10c/flatpak-build.log).
  The installed disposable GNOME account reported OSTree commit
  `38ff628923310ca9b76a9ed7250475f2fff19cba5ab35960dd0e48f7465dcb61`.
- Guest: private compatibility revision 39, unchanged from the preceding
  checkpoint. `SHA256SUMS` SHA256 is
  `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`;
  `guest-manifest.json` is
  `a572cdac779f871c38ee4e2048204e131ea34195443815a336256aa7904e04a4`;
  `vmlinuz-linux` is
  `8ebc2c71271e000540f0a7545afd8e73504fcb724671f3d38c720b241842b901`;
  `initramfs-linux.img` is
  `3247b8993430da428b2d0a219bea2786ebff461cfb83255680c654095f37766a`;
  `rootfs.ext4` is
  `313839548ae9ceec2905a5d5a9535aa13d7dca97faf4539c90811cfc00e5ce2a`.
  The installed GNOME test account's guest install state and boot files matched
  these pins. The private fixture was served only through guest loopback port
  18090 in the disposable desktop VM.
- All principal artifact hashes, including logs and screenshots, are in
  [`phase10c/artifact-sha256.txt`](phase10c/artifact-sha256.txt).

## Change and reason

The GNOME Wayland bridge creates a Remote Desktop portal session, selects only
pointer permission (`types=2`), calls `Clipboard.RequestClipboard` before
`RemoteDesktop.Start`, requires `clipboard_enabled=true`, and keeps the session
open for synchronization. It never calls the portal's remote input methods.
The permission dialog explicitly shows Remote Interaction and Clipboard Access;
the broader permission is explained in Settings and About. The restore token is
written to a mode 0600 file under the app's private config directory.

An installed trial with `types=0` exposed a GNOME 46 behavior: Clipboard Access
could be enabled, but Share remained disabled. Requesting pointer permission
made Remote Interaction selectable and Share available after the user selected
it. That is a necessary consent step on the tested GNOME portal, not evidence
that Try Omarchy controls the host pointer.

The first successful grant still failed to copy host text. A private D-Bus
monitor showed GNOME emitting `mime_types` inside a one-field D-Bus struct.
The bridge accepted only a plain string array. `linuxPortalMIMETypes` now
accepts both forms, and the private-bus regression emits the observed wrapped
form. The normal host-to-guest clipboard polling then saw the new selection.

## Installed GNOME workflow observed here

The test host was a disposable Ubuntu 24.04.5 GNOME Wayland account in the
private devbox desktop VM. GNOME Shell was 46.0 and
`xdg-desktop-portal-gnome` was 46.2. All clicks and screenshots were made
through the task's invisible `gnome-vm` omabox and its VNC viewer. The VM was
not Brandon's desktop, session bus, clipboard, audio, camera or input device.

1. The Phase 10D bundle installed through `flatpak --user install --bundle`.
   The test launch used `-dir /home/tester/matrix/data`, test SSH options and
   `--filesystem=/home/tester/matrix` to reuse a disposable personal-account
   guest and allow the clipboard fixture files. This was an installed GUI and
   real guest boot, but it was not a clean end-user install route.
2. GNOME showed the consent dialog. Remote Interaction and Clipboard Access
   were both enabled by an explicit click before Share. See
   [`gnome-consent-visible.png`](phase10c/gnome-consent-visible.png),
   [`gnome-focus-tab.png`](phase10c/gnome-focus-tab.png) and
   [`gnome-after-share.png`](phase10c/gnome-after-share.png).
   The guest booted to its usable desktop. The app logged
   `clipboard: background sync=true; explicit file drops enabled`.
3. After bundle reinstall and another start, the saved portal token was used
   without a second prompt. The private token file had mode 0600. Text with
   accents and CJK characters, PNG, and a file passed host to guest and guest
   to host against the live r39 guest. The same six checks passed after a later
   clean shutdown and restart. The retained output is
   [`gnome-roundtrip-reconnect.log`](phase10c/gnome-roundtrip-reconnect.log).
   These files were inside the test-only Flatpak filesystem grant. An arbitrary
   external file and Documents FileTransfer portal grant remain unproven.
4. Clicking GNOME's orange Remote Desktop session indicator ended portal
   access. The app showed “GNOME clipboard access ended. Restart Omarchy to
   request access again. File drops remain available.” The guest continued
   running. See [`gnome-after-repeated-fail.png`](phase10c/gnome-after-repeated-fail.png).
   A subsequent copy did not sync, as expected after revocation. A clean guest
   shutdown and app restart reestablished consented synchronization and passed
   all six checks again. The session stop did not erase the persistent grant.
5. The guest was shut down cleanly at 08:51 UTC. The desktop VM was powered off
   cleanly; its qcow2, private guest disk, fixture, source snapshots and logs
   remain. The task `gnome-vm` omabox was stopped. The complete launcher log
   through shutdown is [`gnome-shell.log`](phase10c/gnome-shell.log).

The private D-Bus regression covers pointer-only selection, Clipboard request,
wrapped MIME notification, denial, selection revocation and restore-token
reuse. It does not replace the installed GUI observations above. A real Cancel
in the initial GNOME dialog was not separately exercised; the prior `types=0`
trial timed out after Share stayed disabled. This distinction remains in the
acceptance ledger.

## Regression and compatibility

- Focused private D-Bus portal test passed after the wrapped MIME fix.
- Full Linux race suite passed with QEMU 11.1.1 and matching `qemu-img` on PATH
  inside a network-isolated omabox. Both previously excluded saved-session
  tests and the portable backup materialization test ran. The sole skip was
  `TestUSBRealRuntimeControllerAndMissingDevice`, which requires the Windows
  runtime's custom `usb-host` property. Exact log:
  [`full-linux-race-final.log`](phase10c/full-linux-race-final.log), SHA256
  `240c550741b60b08d27177d1e39ecb62d2955f466786d12a68dee683409cc9f1`.
- Linux `go vet ./...` passed on devbox. Windows amd64 cross-build,
  `go vet -unsafeptr=false ./...` and Windows test compilation passed on devbox
  with Go 1.27.1. The Windows exe SHA256 is
  `b25f786ecd1472b53276fab4e725b06fe06982a8fb8e990b6e5759af9fa21bae`;
  the test exe SHA256 is
  `b875596523cb3708f17409572fe8a81f789e065670a1625030ed5d31c297351e`.
  Native Windows tests remain an unavailable CI gate until an authorized run.
- `git diff --check` passed. The three accepted Phase 8B corrections and the
  Phase 9 microphone retry fix remain in the source; their focused regressions
  passed in the complete suite. The separate correction evidence records their
  prior installed workflows.

## Other lifecycle and release gates

This installed GNOME check did not establish a normal new install to first
desktop. It reused a personal guest, a private loopback release fixture and
test-only launch options. The current app still needs an approved Linux guest
distribution endpoint and independent checksum policy before a public install
can work. The default loopback fixture remains private.

The Phase 10 recovery GUI now offers backup, restore-as-copy and diagnostics
in source. Diagnostics were saved through an earlier installed GNOME bundle;
backup/restore, confirmed reset and relocation have not passed an installed
end-to-end personal VM workflow. Uninstall, retained data, reinstall,
reattachment, deletion of disposable app-owned data, app update rollback and
guest interrupted-update recovery remain open. No shared-folder contents were
deleted. The existing guest update to r39 was verified in this same desktop
VM before the Phase 10D portal build, but that is inherited Phase 10B evidence,
not an exact Phase 10D update rehearsal.

The exact Phase 10D bundle has GNOME Wayland portal and guest boot evidence,
not an Omarchy, KDE Wayland, X11, Intel/NVIDIA and sustained-use matrix. Earlier
matrix results belong to earlier bundles. Linux camera capture, external
file-drop grants, arbitrary FileTransfer clipboard grants, host keyboard-group
tracking, LAN mode, approved host-app launching, USB management, Linux host
authentication and the remaining near-parity decisions are open. Battery
hardware acceptance, real audio/microphone/camera, suspend, monitor changes
and the physical laptop pass remain explicitly deferred.

The host `/data` volume filled during this run. A partial local bundle copy
created by this task was removed; all pre-existing `/data` evidence was kept.
The final bundle and new evidence are under this repository on the `/home`
volume and also on devbox. Do not treat the full `/data` volume as permission
to remove unrelated files or VM disks.

## Recovery and next work for Astra

The Phase 10D source can be reconstructed from the unchanged HEAD and the
retained source-delta chain ending in `phase10c/source-delta.tar.gz`. The exact
Flatpak, build log, test log and screenshots are beside this report. The GNOME
desktop VM disk is `/workspace/try-omarchy-linux/desktop-matrix/desktop.qcow2`
on devbox; the personal guest and consent token remain inside it. The private
guest r39 fixture is `/workspace/try-omarchy-linux/phase8b-guest-r3`. None of
these should be replaced by a latest rebuild during review.

Next, test a real denied GNOME prompt and revoked persistent permission, then
complete external file-drop and FileTransfer grants without a test-only
filesystem allowance. Continue Phase 9 camera/battery and remaining host
integration work. Complete the normal install/uninstall/reinstall and recovery
GUI lifecycle, update interruption and rollback, then freeze a new exact
candidate for the full desktop and GPU matrix. Run native Windows CI only
after publication-related actions are authorized, and defer real-device checks
until Brandon explicitly authorizes the late physical pass. Publication still
needs a separate instruction.
