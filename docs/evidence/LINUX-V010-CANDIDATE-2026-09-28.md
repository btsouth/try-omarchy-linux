# Linux v0.1.0 guest and release rehearsal

Observed 2026-09-28 in a disposable Ubuntu GNOME VM on devbox. The guest was
then published as the [linux-v0.1.0 pre-release](https://github.com/btsouth/try-omarchy-linux/releases/tag/linux-v0.1.0).
The app itself is not released.

## Exact identities

| Item | Identity |
| --- | --- |
| Source | `2cf79cc` on `master` |
| Guest series | Shared patches 0001 through 0102, Linux patches 0103 through 0107, lock refresh 0108; builder head `2436ce9e719701db2eb100e4257395226d5133c6`; compatibility revision 43 |
| Guest `SHA256SUMS` | SHA256 `3ba8b875bc9ffff3dd223367a7c00e1e1e9d3b77a314bb147bba9b4361aeba73`, [contents](linux-v010/guest-SHA256SUMS) |
| Guest release URL | `https://github.com/btsouth/try-omarchy-linux/releases/download/linux-v0.1.0`, pinned in `app/linux_release_linux.go` |
| Flatpak | Bundle SHA256 `823fcaff98a31df76f912df18325cbbf4777e15e848ea217b1090bb01b1d5034`, OSTree `0d129e0fc27f40f0346c0cfcc0159dc47102711718dee0d139a081817e088dfe` |
| Test VM | New child `/workspace/try-omarchy-linux/linux-r43-test/desktop.qcow2` over the preserved Phase 10M overlay, whose SHA256 still starts `3a7f1d962d1d6fd1` |

The release assets are the files in `SHA256SUMS` except the uncompressed
`rootfs.ext4`. The largest, `rootfs.ext4.zst`, is 1.8 GB, under GitHub's 2 GiB
asset limit.

## Build

The Arch repositories had moved since the shared package lock, so the builder
refused the transaction. Patch 0108 refreshes the lock with the repository's
own procedure (automake, coreutils, fastfetch, glibc, imagemagick,
inotify-tools, libvips and python-dbus). `build-guest.sh --contract-only`
passed all 206 guest unit tests locally. On devbox the build ran the same
steps without the unit tests, which assume a non-root user, and with Docker
host networking because the bridge network could not resolve Arch mirrors.
Every file matched `SHA256SUMS`. The release smoke test booted the image,
reached a usable trial account and reported compatibility revision 43 and all
image facts ([result](linux-v010/guest-smoke.txt)).

## Results in the installed app

The launcher was run against a local mirror that serves the same path layout
as GitHub releases, over loopback.

- **Upgrade.** A retained personal VM on portal storage at revision 39 staged
  the new image, booted to the desktop and moved the disk to revision 43. The
  personal file `phase10k-keep.txt` kept SHA256 `0a1ce2ae...`.
- **Missing update bookkeeping, fixed.** Linux never confirmed or rolled back
  guest updates, so the old 7.5 GB image stayed as `guest.previous` and a
  failed image would never roll back. The launcher now confirms an update when
  guest userspace reports ready and restores the previous image at the next
  start when it was never confirmed. In the installed app, the unconfirmed
  update restored revision 39 with a [notice](linux-v010/rollback-notice.jpg);
  the next launch reapplied the update and logged
  `guest update v0.1.0 confirmed after userspace reported ready`, removing
  `guest.previous` and the update state.
- **Fresh install.** Default location, instant trial and no shared folder
  downloaded and verified the guest from the mirror and reached the
  [desktop](linux-v010/fresh-install.jpg). A second GPU boot also rendered.
- **Version.** The app now reports its own version, v0.1.0, instead of the
  Windows launcher's, and reads `linux-vX.Y.Z` release tags.

## Published release

The nine assets were uploaded to a draft and published as a pre-release on tag
`linux-v0.1.0` at `6a1e646`. An anonymous download of `SHA256SUMS` from the
pinned URL hashed to the pinned digest, and the sampled assets matched. With
no flags, a fresh default-location trial install downloaded the guest from
GitHub in about 80 seconds, verified it and reached the
[desktop](linux-v010/github-install.jpg); its install receipt records the
GitHub URL and pinned digest. A returning launch downloaded nothing and
rendered on GPU with no virgl errors.

## Real black guest surface

After the upgrade, every GPU boot of that personal disk showed a black
desktop with only the bar: the rolled-back revision 39 boot and two revision
43 boots. QEMU logged the same failure as Phase 10K: `virtio_gpu_virgl_process_cmd:
ctrl 0x106, error 0x1200` and `context error reported 9 "quickshell" Illegal
resource`. The graphics warning appeared each time
([screenshot](linux-v010/real-black-warning.jpg)); after the rollback notice
it waited in the queue and appeared when the notice closed. Following its
advice, Settings then Software rendering booted the same disk to a normal
[desktop](linux-v010/software-recovery.jpg).

The first boot of that disk on the new image rendered normally, and fresh
trial disks rendered on all four GPU boots, so the failure was intermittent
rather than tied to one image or disk.

### Cause and fix

Restarting only the Omarchy shell in a black guest drew the wallpaper
normally, so no file on the disk was broken. The guest kernel logged
`response 0x1200 (command 0x106)`, a rejected `RESOURCE_ATTACH_BACKING`. With
QEMU guest-error logging enabled, a failing boot recorded
`virtio_gpu_create_mapping_iov: nr_entries is too big (19715 > 16384)`. The
6016x3384 wallpaper texture is about 81 MB. When guest memory is fragmented,
its backing needs more page runs than QEMU's fixed limit of 16384, QEMU
rejects the backing, and the shell draws into an unbacked resource. Phase
10K's black surface logged the same guest error.

Two changes address it. Linux QEMU now accepts up to 262144 entries, 1 GiB of
4 KiB pages, through `runtime-build/linux/patches/qemu/0106-allow-larger-virtio-gpu-backing-lists.patch`.
The Linux launcher also passes `-d guest_errors`, so such rejections reach the
VM's `qemu.log`. Before the patch, the affected disk was black on 5 of 7 GPU
boots. With the patched Flatpak (OSTree `266385f34805cb255f5ceda1b0e80fdbd79ca0764270c3ada6f9e57b57ffa4af`)
it rendered on 8 of 8 GPU boots with no virgl errors and an empty
`qemu.log` ([screenshot](linux-v010/black-surface-fixed.jpg)). The personal
file kept its hash. This VM has no host GPU; QEMU rendered with software GL.

The Windows runtime's QEMU fork has the same limit and may need the same
change.

## Not tested

- The fixed QEMU on a real GPU.
- Guest image rollback after a genuinely broken image; the rollback here
  followed an update that had booted but was never confirmed.
- A personal-account fresh install, and other desktops.
