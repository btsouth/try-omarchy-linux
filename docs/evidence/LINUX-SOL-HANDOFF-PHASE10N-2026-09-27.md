# Phase 10N private backup and restore evidence

Observed 2026-09-27 in a disposable Ubuntu 24.04 GNOME Wayland account inside
the devbox desktop VM. This is a local checkpoint, not a release acceptance or
a physical laptop result. No branch, bundle or endpoint was published.

## Identity and recovery state

- Starting checkout: `linux-core`, HEAD
  `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`, with pre-existing and new
  uncommitted work preserved.
- Build source: `/home/bts/.cache/try-omarchy-phase10n-source`, mirrored byte
  for byte to `/workspace/try-omarchy-linux/phase10l-graphics-src` on devbox.
  The [source delta](phase10n/source-delta.tar.gz) from the Phase 10J snapshot
  has SHA256 `31466d6de3d5bb41ae92ed7c1b83741b32a5e362631a58b115f7e9a631813aa8`.
  Its [manifest](phase10n/source-delta.json) and
  [reconstruction check](phase10n/reconstruction-check.txt) record 745 files.
- Installed [Flatpak bundle](phase10n/phase10n-portal-backup-final.flatpak):
  SHA256 `04947bd59e6a5764a9f68747ea8c6dbb24d6ccfe497d8dbc27adc12f593fbc7d`,
  OSTree `4ed78a194eabf9086543716658e1435c8feda662bfab64b48ccf799f0b5e8fe2`.
- Guest compatibility revision 39. Trusted `SHA256SUMS` SHA256
  `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`.
  Kernel SHA256 `8ebc2c71271e000540f0a7545afd8e73504fcb724671f3d38c720b241842b901`;
  initramfs SHA256 `3247b8993430da428b2d0a219bea2786ebff461cfb83255680c654095f37766a`.
- Stopped parent overlay on devbox:
  `/workspace/try-omarchy-linux/phase10k-retained/desktop-overlay.qcow2`
  SHA256 `aeb34bf9709c64a58a1b3b96d809180955c5a8716e3dc9603598bf9ac0817fc8`.
  Stopped child overlay:
  `/workspace/try-omarchy-linux/phase10l-backup/desktop-overlay.qcow2`.
  The disposable active grandchild was
  `/workspace/try-omarchy-linux/phase10m-capacity/desktop-overlay.qcow2`,
  extended to 160 GiB sparse to hold independent restore copies. Do not flatten
  or overwrite these overlays when reviewing.

## Installed workflows observed

The Phase 10L child first used the installed Phase 10J GUI to back up the
retained default-location personal VM and restore a first copy. Its personal
guest file survived and the restored VM booted. This earlier result led to the
Phase 10N portal-specific fixes below. It is separate from the final bundle
check.

The installed Phase 10N GUI then backed up that first portal-reattached copy.
GNOME Files granted an external backup destination, and the GUI completed with
a readable archive filename in the result. Archive:
`/home/fresh10i/BackupFixture/try-omarchy-backup-20260928-002105.360313460.zip`,
inode 1310897, 6,577,126,774 bytes, SHA256
`1379442b7e3e8257a5ca7310ff5c707a6d8aeb94dbd547a51c8f10951637991f`.
Its eight manifest entries included the guest boot pair and a disk with SHA256
`04c6b1911fd9551dfbc04324cd616f9624c7877c15684d380e1f82d0c2e412ac`.
There were no shared-host-folder entries. See the
[archive inspection](phase10n/reattached-backup-archive.txt) and
[GUI result](phase10n/reattached-backup-progress3.png).

The installed GUI restored that archive into a separate second copy under
`/home/fresh10i/SecondRestoreFixture/try-omarchy-restored-20260928-003300.203459741`.
Before boot, its 24 GiB disk SHA256 matched the archive manifest exactly.
Its kernel and initramfs matched the trusted guest pair. The three preserved
disk inodes were 1123315 for the original, 4980753 for the first restore and
9699343 for the second. See [integrity output](phase10n/second-restore-integrity.txt)
and [GUI result](phase10n/restore-progress3.png). The GUI reattached the
second copy, reached a visible Omarchy desktop, and its guest Foot terminal
reported `0a1ce2ae858b6d1c20803b2d34a70f1c938db914c5c7b457966f4bb7e6c922b5`
for `~/Documents/phase10k-keep.txt`, matching the original file. See the
[reattached home](phase10n/reattached-second-home.png),
[desktop](phase10n/second-restored-desktop.png) and
[guest file result](phase10n/second-restored-sentinel.png). The guest was
powered off cleanly through its private QMP socket.

A 24 GiB ext4 loop fixture mounted only inside the disposable outer VM had
about 22.2 GiB available. The installed GUI refused backup before output,
reporting that 32.4 GiB was needed. The original and first restored disk
inodes remained fixed. See [GUI failure](phase10n/lowspace-result.png),
[fixture](phase10n/lowspace-fixture.txt) and
[preservation output](phase10n/lowspace-preservation.txt).

## Corrections and source checks

The first backup attempt from a GNOME document-portal reattachment exposed a
standard `/run/user/1001/doc` symlink, which the move path validator refused.
The validator now permits only that known Flatpak document mount ancestor;
selected data, guest and disk symlinks remain rejected. The next attempt
exposed document FUSE's unsupported `flock`. The fallback applies only to
the portal disk and only for unsupported-lock errors; it checks the private
QMP control socket and running QEMU file descriptors before backup. Focused
portal regression tests passed. The successful installed backup above used
these fixes.

A separate intermittent black virgl surface from Phase 10K remains open. A
nonfatal graphics warning was observed in the installed GUI after injecting
the exact virgl failure text into a disposable helper log. It appeared once
and left the VM running: [first warning](phase10l/graphics-warning.png) and
[no repeat](phase10l/graphics-warning-no-repeat.png). This synthetic check
does not prove actual graphics recovery or eliminate the black surface.

The [compatible QEMU script](phase10n/run_compatible_qemu_suite.sh) ran the
full Linux race suite inside the pinned Flatpak SDK with packaged QEMU 11.1.1
and Go 1.27.1. App tests passed in 47.211s and sign-update tests in 1.025s,
including the two saved-session tests: [full log](phase10n/compatible-suite.log)
SHA256 `1e8ced1abf7a3e39850fe4e58c4ef1e1776679ae99c932b74077bc95a793da47`.
The [targeted saved-session log](phase10n/saved-session-compatible.log)
also passed. Host QEMU 8.2.2 failed only those two tests because its QMP
rejects `exit-on-error`; [host log](phase10n/full-qemu82.log). Linux vet,
Windows vet with `-unsafeptr=false`, Windows cross-build and Windows test
compilation passed: [cross-check log](phase10n/cross-checks.log). Native
Windows tests still require the Windows CI job; nothing was pushed to run it.

## Open result found after this checkpoint

An installed restore cancellation removed its staging copy and kept all three
VM disks, but the Phase 10N helper stayed on “Cancelling setup...”. Phase 10O
made the home actions usable but exposed a disabled Close button. Phase 10P
corrects the complete state transition and is the bundle to review next.

Guest artifact update and rollback, full graphical installation, physical
hardware, and the exact Omarchy/GNOME/KDE/X11 candidate matrix remain release
gates. The home screen still displays an opaque document-portal storage path
after reattachment. Portal lock fallback deserves independent safety review.
