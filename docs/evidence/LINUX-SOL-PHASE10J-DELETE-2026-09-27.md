# Phase 10J private default-VM deletion, 2026-09-27

## Result

The installed Linux home now offers **Delete this VM...** only for the
app-owned default VM. A separate confirmation names the exact data folder and
states that guest files will be lost while shared host folders and external
backups remain. Keep leaves the disk in place. Confirmed Delete removes only
the `vm` and `guest` contents owned by this default installation and its
provision-mode marker. It keeps Settings, diagnostics and anything outside
those folders. An external selected VM is not eligible.

In a disposable Ubuntu GNOME Wayland account, the final installed private
Flatpak passed Keep, then Delete with a real saved 24 GiB personal disk and a
portal-selected shared folder. The VM and guest directories disappeared; the
shared sentinel SHA256 and saved folder choice remained. Launch then returned
to the storage chooser, and Cancel did not recreate the VM. The private app
package also rolled back to the Phase 10H bundle and re-upgraded to Phase 10J
without changing that retained shared file or recreating a disk. This is a
partial app package rollback check, not the full update/recovery gate.

## Exact identity and recovery state

- Checkout `/home/bts/Projects/try-omarchy-linux-core`, branch `linux-core`,
  starting HEAD `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`. Changes are
  uncommitted, local and private. The prior dirty worktree, disks and evidence
  were preserved.
- Parent source snapshot `/home/bts/.cache/try-omarchy-phase10h-source` and
  new build source `/home/bts/.cache/try-omarchy-phase10j-source`, mirrored at
  `/workspace/try-omarchy-linux/phase10j-delete-src` on devbox. The
  [reconstructable delta](phase10j/source-delta.tar.gz), SHA256
  `2fb30e4471b1a4106eb98d6ce095a1984f367f3a8a8125c8bce4593854475d5e`,
  changes or adds five files, removes none, and yields 739 source files. Its
  [manifest](phase10j/source-delta.json) and [reconstruction check](phase10j/reconstruction-check.txt)
  tie it to the Phase 10H source and bundle.
- [Final private Flatpak](phase10j/phase10j-delete-final.flatpak) SHA256
  `618eaf8af04e72e71619002e80d014f372aa322e38863f9c2d2f52e5168df242`.
  Installed OSTree commit
  `500fb2e388c44382b90dc848392606f806a258f93457cc4bc9294aaf42ef1483`.
  The [build log](phase10j/flatpak-build.log) and
  [installed upgrade](phase10j/installed-upgrade.txt) record the build and
  private package install. The prior Phase 10H bundle SHA256 is
  `1ebd5fab35c0c8e9901f533062b6856faa0bf00c4b50de82ae2d893126a85649`
  with OSTree commit
  `0332b6a050e9547fe19c6f71b594b86eb455565ae155e1fb22e7237291ba2514`.
- Guest source remains compatibility revision 39, `SHA256SUMS` SHA256
  `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`.
  The Phase 10I parent overlay preserved its real personal disk, and the new
  Phase 10J child overlay was the only disk changed by Delete. Phase 10I
  recomputed the installed guest kernel and initramfs hashes and booted it
  twice. This slice did not build or upgrade a guest.
- Disposable child overlay
  `/workspace/try-omarchy-linux/phase10j-delete/desktop-overlay.qcow2` backs
  onto the stopped Phase 10I overlay. After clean outer-VM shutdown its SHA256
  was `a94c728e5c74d221a4433df8156e7afbb2d7f60000ab8bacd62afe879bf450c6`.
  The [launch script](phase10j/launch-desktop-vm.sh) and
  [final overlay record](phase10j/outer-vm-final.txt) preserve the fixture.
  The child is recoverable, and the untouched Phase 10I parent still contains
  the personal disk. The task omabox and VNC tunnel are stopped.

## Implementation and observed workflow

`app/delete_default_linux.go` checks that the data path ends in the Linux
default folder name, no saved external-location pointer exists, and `disk.raw`
is regular. Before removing anything, it checks the `vm` and `guest`
directories and every entry against a list of app-created regular files. It
refuses symlinks, unexpected files, unreadable Settings and an overlapping
shared folder. Deletion then removes those known files and empty directories;
Settings and other top-level contents remain. An error during removal is
reported in the home UI. This preflight protects against a common accidental
external-data deletion; it is not a transactional crash-recovery mechanism.

| Check | Evidence and result |
| --- | --- |
| Upgrade with retained VM | The Phase 10I child started with the personal disk inode `1123315`, capacity 24 GiB in [pre-upgrade state](phase10j/pre-upgrade-state.txt). Private Flatpak [upgrade](phase10j/installed-upgrade.txt) installed the new commit. The [home](phase10j/upgraded-home.png) found the disk and showed Delete. A post-upgrade guest boot was not run before the deliberate deletion. |
| Keep | The [confirmation](phase10j/delete-confirmation2.png) named the exact default path and described the loss. Keep returned to the [home](phase10j/kept-home.png). The [state](phase10j/keep-state.txt) still showed disk inode `1123315` and the shared sentinel hash. |
| Shared folder | GNOME's [folder picker](phase10j/share-picker.png) granted `/home/fresh10i/SharedFixture` through the portal. The installed GUI saved that choice. [Before deletion](phase10j/before-delete-state.txt), Settings held document path `/run/flatpak/doc/c702a217/SharedFixture`; the external sentinel SHA256 was `e10d593d3a97ca60238c28a7e5dafd48d1772592c580fa5a7b1ddef7892765b2`. |
| Confirmed Delete | The installed [confirmation with the shared folder selected](phase10j/delete-with-share-confirm.png) again named the default VM path. Clicking Delete returned an explicit [success message](phase10j/after-delete-home.png). [Filesystem state](phase10j/after-delete-state.txt) showed `vm`, `guest` and `provision-mode` absent, Settings with the same share path, the same external sentinel hash and free space restored from 34 to 47 GiB. |
| Fresh state after Delete | Launch opened the [location chooser](phase10j/post-delete-launch.png). Cancel left no VM directory in [state](phase10j/post-delete-cancel.txt). |
| Package rollback and re-upgrade | Private `flatpak install --reinstall` returned to the Phase 10H commit in the [rollback record](phase10j/rollback-to-phase10h.txt); its [home](phase10j/rollback-home.png) showed no VM. Reinstalling the Phase 10J bundle restored commit `500fb2...` in the [re-upgrade record](phase10j/reupgrade-to-phase10j.txt). The [final home](phase10j/final-reupgraded-home2.png) still showed no VM, and Settings retained the portal share and external sentinel hash. |

## Regression checks and limits

- `TestLinuxDeleteDefaultVMKeepsSharedAndPreferences` checks that the VM and
  guest artifacts are removed while Settings, the shared file and ability to
  choose storage again remain. Two more tests refuse unknown guest files,
  selected external data, overlapping shares and symlinks before deletion.
  The devbox [race checks](phase10j/source-checks.log) passed the full Linux
  `go test -race -count=1 ./...`. The [explicit cross-check log](phase10j/explicit-cross-checks.log)
  passed Linux vet, Windows vet with the repository's `-unsafeptr=false`
  setting, Windows cross-build and Windows test compilation. Native Windows
  tests were not run.
- Deletion has a preflight but no resumable transaction if the process or
  storage fails mid-removal. The GUI reports an error and retains the Phase
  10I parent in this test, but real users would need a backup for recovery.
  Interrupted deletion and low-space behavior still need release acceptance.
- This app rollback happened after intentionally deleting the VM. It proves
  two private Flatpak commits can replace one another while Settings and a
  shared folder remain. It does not prove that either update or rollback
  preserves a personal disk or matched guest boot artifacts. No guest update,
  signature or checksum-failure rehearsal occurred.
- The private bundle still points at a loopback guest fixture. Graphical
  Software installation/removal, public artifact policy, full Phase 10
  update/recovery matrix, Phase 11 exact-candidate matrix, Windows CI and
  physical checks remain open. No publication occurred.

## Next actions

Rehearse app and guest upgrade, verification failure, interrupted download,
rollback and recovery against a retained personal VM and sentinel files.
Finish GUI backup/restore, relocation, reset and low-space recovery. Then
freeze an exact private candidate for the full desktop and regression matrix.
