# Phase 10E private file portal checkpoint, 2026-09-27

## Verdict

Installed GNOME 46 Wayland file copy and paste now works in both directions
with a host file outside the Flatpak's direct filesystem grant. The Documents
FileTransfer portal supplies the grants. This is a tested improvement to the
Phase 10D GNOME clipboard checkpoint. The original native GNOME Files drag
was incorrectly classified as a failure because success did not appear in the
app log. The [Phase 10F correction](LINUX-SOL-PHASE10F-DROP-CORRECTION-2026-09-27.md)
found that file in guest Downloads and repeated native drops, a portal chooser
grant, and cancellation on this exact installed bundle. Other Phase 9 and
release gates remain open. No publication, physical desktop test, or main
Windows checkout change occurred.

## Exact candidate and recovery state

- Checkout: `/home/bts/Projects/try-omarchy-linux-core`, branch `linux-core`;
  unchanged starting HEAD `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`.
  Earlier uncommitted work and evidence remain in place.
- Build source: `/workspace/try-omarchy-linux/phase10e-fileportal-src` on
  `devbox`, mirrored at `/home/bts/.cache/try-omarchy-phase10e-source`.
  The reconstructable delta from the retained Phase 10D source is
  [`phase10e/source-delta.tar.gz`](phase10e/source-delta.tar.gz), SHA256
  `d531c8e51e2a621d2f28bceb5c865454c3047e7147145f668cd66c6ff1412d15`.
  Its [manifest](phase10e/source-delta.json) identifies seven changed or new
  files, no removals, the parent delta and bundle, and each file hash. Every
  archived file hash and its current worktree counterpart was checked.
- Final Flatpak: [`phase10e/phase10e-final.flatpak`](phase10e/phase10e-final.flatpak),
  SHA256 `0c8386ca78a5b1369f9269bc992a5d09c6bda0880c594adaee6fc895d785a05a`.
  The devbox output has the same hash. Installed OSTree commit in the disposable
  GNOME account: `ebf133d71595c92892b03d6c24a53443ff4a07eb1a5936f0b600e83c378bddc8`.
  The [build log](phase10e/flatpak-build-final.log) is retained.
- Guest: same private compatibility revision 39 as Phase 10D. The inherited,
  previously checked `SHA256SUMS` hash is
  `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`;
  `guest-manifest.json` is
  `a572cdac779f871c38ee4e2048204e131ea34195443815a336256aa7904e04a4`;
  kernel `8ebc2c71271e000540f0a7545afd8e73504fcb724671f3d38c720b241842b901`;
  initramfs `3247b8993430da428b2d0a219bea2786ebff461cfb83255680c654095f37766a`;
  rootfs `313839548ae9ceec2905a5d5a9535aa13d7dca97faf4539c90811cfc00e5ce2a`.
  These guest hashes were carried from the Phase 10D report, not freshly
  recomputed in this run. The existing personal guest booted and shut down
  cleanly on the new host bundle.
- Isolated desktop: copy-on-write overlay
  `/workspace/try-omarchy-linux/phase10e-gnome-live/desktop-overlay.qcow2`
  over the retained GNOME matrix base on devbox. It contains the installed
  Flatpak, consent state, personal guest, and disposable test files. Guest and
  desktop VM both shut down cleanly; QEMU removed its PID file. Only this task's
  `phase10e-gnome` omabox and VNC tunnel were stopped. Other boxes and disks
  were left alone.

## Implementation

The GNOME clipboard bridge now publishes guest files using Documents
FileTransfer `StartTransfer` and batched `AddFiles`, offers the transfer key as
`application/vnd.portal.filetransfer`, and stops old transfers when the
selection changes or closes. Host file clipboard grants use `RetrieveFiles`.
The file source validator resolves only Flatpak's document-mount link before
the archive's usual no-link validation; it rejects escapes and unrelated
symlinks. File-drop chooser results use the same validator. Grant and transfer
errors set visible clipboard or file-drop status.

The first built bundle failed a real incoming copy with `linked paths cannot
be moved: /run/user/1000/doc`, because Flatpak's document mount itself is a
symlink. That bundle, error screenshot and historical shell log are retained.
The final source fixes that specific case while keeping ordinary symlinks
rejected. No shared-folder contents were deleted or moved.

## Verification observed in this run

1. `go -C app vet ./...` passed. The five focused tests in
   [`phase10e/focused-tests.log`](phase10e/focused-tests.log) passed inside a
   throwaway omabox and private D-Bus session. They cover MIME/URI handling,
   outgoing transfer lifecycle and cleanup, incoming retrieval success/denial,
   external drop grant requirement, document-root confinement, non-ASCII names,
   and escape symlinks. The private D-Bus test receiver cannot decode godbus's
   UnixFD array, so that test injects the `AddFiles` call. The installed
   roundtrip below exercises the real portal.
2. The final Flatpak launched the retained personal r39 desktop in the private
   GNOME VM with only `--filesystem=/home/tester/matrix/data` for the existing
   VM data. A direct sandbox `test -e` on
   `/home/tester/Documents/TryOmarchy-external/host-external-世界.txt` exited 1.
   This establishes that the outside test file had no direct Flatpak grant.
3. In host GNOME Files, Copy on that outside file reached the guest clipboard
   through the portal. Guest `wl-paste` showed a file URI in the received
   transfer cache. Guest Files Paste created
   `/home/omarchy/host-external-世界.txt`; its SHA256 was
   `50e11ef328e3808b102798cbaa5b06067a4d766463b8c4a9c843ec403a0cc2d3`,
   matching the outside host fixture. See
   [copied source](phase10e/final-external-copied.png) and
   [guest Files after paste](phase10e/host-to-guest-gui.png).
4. In guest Files, Copy on `guest-phase10e-世界.txt` reached host GNOME Files.
   Paste into an outside host Documents folder created the correctly named
   file with the same SHA256. See
   [host Files after paste](phase10e/guest-to-host-gui.png).
5. A separate `host-drop-世界.txt` was dragged from host GNOME Files over the
   visible VM. The [hover screenshot](phase10e/drop-hover.png) shows a copy
   cursor. At the time, no chooser or success line appeared in the
   [installed shell log](phase10e/installed-shell.log), and the drop was
   incorrectly called unproven. Phase 10F found the file in guest Downloads
   with the time of this drag and repeated the workflow using fresh files and
   matching hashes. GNOME supplied an implicit portal grant, so no chooser
   was expected for this native Files drop.
6. The guest stopped with `guest stopped (poweroff)` and the app unit became
   inactive. The desktop VM then shut down. No host desktop bus, clipboard,
   camera, microphone, speaker, or input device was used.

The installed shell log also has one `portal clipboard: unsupported MIME list
type <nil>` line during an owner-change notification. Copy/paste still worked
after it. This needs triage if it recurs or affects a transfer; it is not
counted as a failed transfer in this run.

## Remaining release work

- Verify direct drops into guest apps, interruption during transfer, revoked
  Documents grants, reconnect and visible failures on the installed package.
  Phase 10F separately passed native Files drops to guest Downloads, a 64 MiB
  file, chooser grant and clean cancellation on this exact bundle. Windows
  v0.6.0 supports direct drops into guest apps; that outcome remains untested.
- Run interruption, reconnect, revoked-document, and broader
  GNOME/KDE/X11/Omarchy tests against a later exact candidate. Phase 10D has
  separate GNOME consent/revocation evidence; those checks were not repeated
  against this bundle.
- Continue remaining Phase 9 controls and integrations, Phase 10 install,
  uninstall, update and recovery lifecycle, and Phase 11 exact matrix. Native
  Windows CI and owner-authorized physical acceptance remain outstanding.
  No release readiness or publication is claimed.

All local artifact hashes, including screenshots and logs, are in
[`phase10e/artifact-sha256.txt`](phase10e/artifact-sha256.txt). To recover the
same installed state, reopen only the retained devbox overlay. Reconstruct
source by applying the Phase 10D chain then this delta; do not substitute a
fresh rebuild for the recorded bundle.
