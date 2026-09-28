# Private Linux handoff through Phase 10K, 2026-09-27

## State for Astra

Try Omarchy Linux is still a private, uncommitted development candidate. The
latest exact app bundle has installed GUI evidence for first personal setup,
default-VM deletion, retained external data after uninstall, storage regrant,
and app package upgrade/rollback with a retained personal file. This does not
close Phase 10 or establish a release candidate. The latest update rehearsal
also found one intermittent black guest surface on automatic GPU rendering in
the nested Ubuntu GNOME fixture. Software rendering recovered it, but repeated
exact-candidate graphics testing is required.

## Identities and reconstruction

- Repository `/home/bts/Projects/try-omarchy-linux-core`, branch `linux-core`,
  starting HEAD `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`. Preserve all
  uncommitted source and evidence. No commit, push, PR or publication occurred.
- Latest Phase 10J [source snapshot delta](phase10j/source-delta.tar.gz) SHA256
  `2fb30e4471b1a4106eb98d6ce095a1984f367f3a8a8125c8bce4593854475d5e`.
  The [manifest](phase10j/source-delta.json) and
  [reconstruction check](phase10j/reconstruction-check.txt) derive it from
  `/home/bts/.cache/try-omarchy-phase10h-source`. Built source is retained at
  `/home/bts/.cache/try-omarchy-phase10j-source` and mirrored on devbox at
  `/workspace/try-omarchy-linux/phase10j-delete-src`. Phase 10K changed no
  source code.
- Latest [private Flatpak](phase10j/phase10j-delete-final.flatpak) SHA256
  `618eaf8af04e72e71619002e80d014f372aa322e38863f9c2d2f52e5168df242`;
  installed OSTree commit
  `500fb2e388c44382b90dc848392606f806a258f93457cc4bc9294aaf42ef1483`.
  Prior Phase 10H [bundle](phase10h/phase10h-storage-final.flatpak) SHA256
  `1ebd5fab35c0c8e9901f533062b6856faa0bf00c4b50de82ae2d893126a85649`,
  OSTree commit
  `0332b6a050e9547fe19c6f71b594b86eb455565ae155e1fb22e7237291ba2514`.
- Guest compatibility revision 39; `SHA256SUMS` SHA256
  `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`.
  Installed kernel SHA256
  `8ebc2c71271e000540f0a7545afd8e73504fcb724671f3d38c720b241842b901`;
  installed initramfs SHA256
  `3247b8993430da428b2d0a219bea2786ebff461cfb83255680c654095f37766a`.
  The compressed rootfs inherited SHA256 is
  `afce576526c826c7fb19eec08988469efe2ab41262c3c7ab253315ad76b6e108`.
  Phase 10I's install receipt pins unpacked rootfs SHA256
  `313839548ae9ceec2905a5d5a9535aa13d7dca97faf4539c90811cfc00e5ce2a`.
- Retained personal-VM update fixture is the stopped devbox child
  `/workspace/try-omarchy-linux/phase10k-retained/desktop-overlay.qcow2`,
  SHA256 `aeb34bf9709c64a58a1b3b96d809180955c5a8716e3dc9603598bf9ac0817fc8`.
  Its Phase 10I parent retains an earlier personal VM; the separate Phase 10J
  deletion child retains the deletion test. They were not overwritten.

## Completed installed workflows and direct evidence

| Workflow | Direct result |
| --- | --- |
| First personal setup | A new account in a disposable Ubuntu GNOME VM installed the Phase 10H private bundle, found its launcher icon, canceled before storage choice without creating a disk, then completed default personal setup to a visible Omarchy desktop. Clean shutdown and relaunch used the saved disk without another guest download. The OS and private fixture were inherited, so this was not a clean OS or graphical Software installation. [Phase 10I report](LINUX-SOL-PHASE10I-FIRST-INSTALL-2026-09-27.md). |
| Uninstall and retained data | Ordinary Flatpak uninstall kept a custom-location VM, guest boot files and shared sentinel. Reinstall plus a GNOME portal folder grant reattached and booted the VM. Installed GUI also recovered after a saved location disappeared or its document export was revoked; `--delete-data` removed app-owned data and saved pointer while retaining the external VM. [Phase 10G report](LINUX-SOL-PHASE10G-REATTACH-2026-09-27.md), [Phase 10H report](LINUX-SOL-PHASE10H-STORAGE-2026-09-27.md). |
| Explicit default data deletion | Latest installed Phase 10J GUI offered Delete only for its app-owned default VM. Keep retained a 24 GiB disk; confirmed Delete removed only VM and guest artifacts. Settings and the portal-selected shared-folder sentinel survived; the next launch offered storage choice and Cancel recreated nothing. [Phase 10J report](LINUX-SOL-PHASE10J-DELETE-2026-09-27.md). |
| App replacement and rollback | A separate retained personal VM booted on Phase 10H, Phase 10J, rollback to 10H, and re-upgrade to 10J. A real guest Documents file kept SHA256 `0a1ce2ae858b6d1c20803b2d34a70f1c938db914c5c7b457966f4bb7e6c922b5` on each visible boot. Disk inode `1123315` and installed kernel/initramfs hashes stayed fixed. One black GPU boot during rollback recovered with Software rendering; a later automatic GPU retry worked. [Phase 10K report](LINUX-SOL-PHASE10K-RETAINED-UPDATE-2026-09-27.md). |
| Clipboard and external files | GNOME Wayland consented Remote Desktop portal, two-way text/PNG/file transfers, external FileTransfer portal grant, and native Files drop to guest Downloads passed in earlier installed workflows. Direct drop into an arbitrary guest app and interruption/revocation paths still need completion. [GNOME portal report](LINUX-SOL-GNOME-PORTAL-2026-09-27.md), [Phase 10E report](LINUX-SOL-PHASE10E-FILE-PORTAL-2026-09-27.md), [Phase 10F correction](LINUX-SOL-PHASE10F-DROP-CORRECTION-2026-09-27.md). |

## Current defects, parity and external gates

- Automatic GPU rendering showed one black guest surface after desktop-ready
  on rolled-back Phase 10H in the nested GNOME VM, with a virgl `quickshell`
  resource error. Software rendering and a later automatic retry worked.
  Cause and frequency are unproven. This is a concrete Phase 11 graphics gate.
- Full GUI backup/restore, reset, relocation, guest updates and verification,
  interruption, rollback and recovery remain incomplete. Low-space, corrupt
  settings and physical removable storage need deeper installed acceptance.
- Camera capture, packaged battery behavior and physical audio/device paths
  remain behind parity or acceptance gates. Linux does not yet match Windows
  for approved host-app launch, portable USB management, live audio route
  switching and certain host authentication integrations. See the dated
  [parity inventory](../LINUX-PARITY.md) for released Windows/Mac comparisons.
- The bundle uses a private loopback guest fixture. Public artifact location,
  independent trust pin, update channel and graphical distribution route are
  unresolved. No publication endpoint was created.
- The Omarchy, GNOME, KDE Wayland and X11 matrix has earlier partial evidence,
  including Intel and NVIDIA isolation, but has not been rerun against the
  Phase 10J bundle and exact guest pair. The full Linux suite on compatible
  QEMU remains open; the two QEMU 8.2 saved-session exclusions do not count.
  Phase 10J source passed full Linux race tests and vet, Windows vet with the
  repository's `-unsafeptr=false` setting, cross-build and test compilation in
  devbox. [Race checks](phase10j/source-checks.log) and
  [explicit cross-checks](phase10j/explicit-cross-checks.log).
  Native Windows tests belong to Windows CI, which is unavailable while this
  work remains local. No physical laptop or real-device acceptance was run.
- Owner scope decisions still needed before publication include Linux LAN or
  bridge networking, approved host-app launch, USB device access, host
  authentication parity and public artifact policy. GNOME consented portal
  clipboard sync was already chosen by the owner and is implemented privately.

## Recovery and next concrete actions

The Phase 10K child is stopped with Phase 10J installed and its personal file
intact. The [launcher](phase10k/launch-desktop-vm.sh) plus mode-0600 task key
at `/home/bts/.cache/try-omarchy-phase10k-ssh` can reopen the fixture.
The Phase 10I parent preserves the earlier personal VM and the Phase 10J
child preserves the deletion outcome. Phase 10H and 10J Flatpaks and source
snapshots remain locally available for rollback or reconstruction.

Next, reproduce or bound the black GPU presentation in the exact candidate;
finish guest update and verification/interruption/recovery, then GUI
backup/restore and relocation with file preservation. Complete graphical
installation and uninstall from a clean OS, freeze exact hashes, run the full
desktop and regression matrix with compatible QEMU, and submit remaining
Windows CI and authorized physical checks. Publication needs a separate owner
instruction after those gates.
