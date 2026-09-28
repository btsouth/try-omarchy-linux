# Phase 10K private app update with a retained personal VM, 2026-09-27

## Result

The installed Phase 10H Flatpak booted a saved personal Omarchy VM. A file
created in the guest's Documents folder then survived replacement with the
Phase 10J Flatpak, rollback to Phase 10H, and re-upgrade to Phase 10J. Each
package version found the same 24 GiB disk. The installed guest kernel and
initramfs, the disk inode and an outer-account sentinel also stayed the same.
The final Phase 10J GUI reached the Omarchy desktop and showed the guest file
with its original SHA256. All package replacements used local private bundles
with the guest powered off.

One automatic GPU boot on the rolled-back Phase 10H package displayed a black
guest surface even though the app log reported desktop-ready. QEMU recorded a
virgl resource error involving `quickshell`. A QMP powerdown completed cleanly.
Choosing Software rendering in the installed Settings GUI produced a visible
desktop with the personal file intact. A later automatic GPU retry on the same
rolled-back package and the final Phase 10J boot both rendered normally. This
single intermittent failure is an open graphics stability finding for the
exact-candidate matrix, not evidence of lost guest data.

## Exact identity and isolation

- Checkout `/home/bts/Projects/try-omarchy-linux-core`, branch `linux-core`,
  starting HEAD `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`. Source is
  unchanged since Phase 10J, local and uncommitted. The
  [Phase 10J source delta](phase10j/source-delta.tar.gz) SHA256 is
  `2fb30e4471b1a4106eb98d6ce095a1984f367f3a8a8125c8bce4593854475d5e`.
- Earlier private Phase 10H [Flatpak](phase10h/phase10h-storage-final.flatpak)
  SHA256 `1ebd5fab35c0c8e9901f533062b6856faa0bf00c4b50de82ae2d893126a85649`,
  OSTree commit `0332b6a050e9547fe19c6f71b594b86eb455565ae155e1fb22e7237291ba2514`.
  Final Phase 10J [Flatpak](phase10j/phase10j-delete-final.flatpak) SHA256
  `618eaf8af04e72e71619002e80d014f372aa322e38863f9c2d2f52e5168df242`,
  OSTree commit `500fb2e388c44382b90dc848392606f806a258f93457cc4bc9294aaf42ef1483`.
- Guest compatibility revision 39. Manifest `SHA256SUMS` SHA256
  `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`.
  The installed kernel SHA256 stayed
  `8ebc2c71271e000540f0a7545afd8e73504fcb724671f3d38c720b241842b901`
  and installed initramfs SHA256 stayed
  `3247b8993430da428b2d0a219bea2786ebff461cfb83255680c654095f37766a`.
  No guest upgrade occurred.
- Disposable devbox outer VM child
  `/workspace/try-omarchy-linux/phase10k-retained/desktop-overlay.qcow2`
  backs onto the stopped Phase 10I first-install overlay, leaving the Phase
  10I parent and Phase 10J deletion child untouched. The child was shut down
  cleanly with the Phase 10J Flatpak installed and its personal guest file
  retained. Stopped overlay SHA256:
  `aeb34bf9709c64a58a1b3b96d809180955c5a8716e3dc9603598bf9ac0817fc8`.
  The [launch script](phase10k/launch-desktop-vm.sh) and
  [final image state](phase10k/outer-vm-final-clean.txt) preserve the fixture.
  A task-only SSH fixture key is retained locally at
  `/home/bts/.cache/try-omarchy-phase10k-ssh` with mode 0600. The omabox and
  VNC tunnel are stopped. No user desktop or device was accessed.

## Observed installed workflow

| Step | Evidence and result |
| --- | --- |
| Baseline | The Phase 10H [installed home](phase10k/baseline-home2.png) found the saved 24 GiB disk. [Pre-upgrade state](phase10k/pre-upgrade-state.txt) records OSTree commit, disk inode `1123315`, guest boot hashes and an outer-account sentinel. The [guest desktop](phase10k/baseline-launch2.png) appeared. In guest Foot, a synthetic personal file was created in `~/Documents`; the [terminal result](phase10k/guest-sentinel-created.png) showed SHA256 `0a1ce2ae858b6d1c20803b2d34a70f1c938db914c5c7b457966f4bb7e6c922b5`. The guest powered off cleanly. |
| Upgrade | Private `flatpak install --user --reinstall` installed Phase 10J. The [state record](phase10k/upgrade-retained-state.txt) showed the new commit with the same disk inode and boot hashes. The [new installed home](phase10k/upgraded-home2.png) found the disk and the [guest terminal](phase10k/upgraded-guest-sentinel2.png) showed the same file hash. The guest powered off cleanly. |
| Rollback finding | The [rollback record](phase10k/rollback-retained-state.txt) showed the Phase 10H commit with the same disk and boot hashes. Its [home](phase10k/rollback-home2.png) found the VM, but one [automatic GPU boot](phase10k/rollback-guest2.png) showed a black surface after desktop-ready. The [logs](phase10k/rollback-black-screen-logs.txt) contain the virgl error. [QMP powerdown](phase10k/rollback-qmp-poweroff.txt) and [app exit](phase10k/rollback-after-qmp.txt) were clean. |
| Recovery | The installed GUI saved [Software rendering](phase10k/rollback-software-saved2.png). The [guest desktop](phase10k/rollback-software-guest.png) and [personal file hash](phase10k/rollback-software-sentinel2.png) returned. After another clean shutdown, automatic graphics was restored in Settings and the [retry](phase10k/rollback-auto-retry-guest.png) rendered normally. |
| Re-upgrade | Phase 10J was installed again with the same inode and hashes in the [final replacement record](phase10k/reupgrade-retained-state.txt). The [home](phase10k/reupgrade-home2.png) found the VM; the [guest](phase10k/reupgrade-guest.png) and [personal file](phase10k/reupgrade-sentinel2.png) were visible. After clean guest and outer VM shutdown, [final state](phase10k/final-retained-state.txt) still had the new commit, matched boot pair, saved disk and outer sentinel. |

The outer sentinel SHA256 was
`a459262932f5d2f219096371a8bfb63b45cc2a2ebf94b92a92cc840701e805e3`.
The saved disk's inode remained `1123315` through every package change. Those
checks alone would not prove guest contents, which is why each version was
booted and the guest Documents file was read in its own terminal.

## Limits and next work

- This proves app package replacement and rollback between two private
  Flatpaks with a retained personal VM. It does not test a guest update,
  signature or checksum failure, interrupted download/apply, or a power loss
  during replacement. The private bundle still uses a loopback guest fixture.
- The black automatic GPU boot was not reproducible on one retry. The exact
  candidate needs repeated GPU boots and pressure testing on GNOME, KDE,
  Omarchy and X11, with a usable recovery path if desktop-ready is announced
  while the window remains black. Software rendering recovered this fixture.
- Graphical installation/removal, GUI reset/move and full backup/restore,
  guest updates, the full compatible-QEMU suite, Windows CI, physical device
  acceptance and distribution policy remain open. No publication occurred.
