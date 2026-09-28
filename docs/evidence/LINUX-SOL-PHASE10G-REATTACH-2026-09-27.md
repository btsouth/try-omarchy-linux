# Phase 10G private install lifecycle: retained VM reattachment, 2026-09-27

## Result

The installed app now offers **Use existing data folder** on its home screen.
It opens the desktop folder portal, checks for a complete Try Omarchy VM, checks
write access and saves the chosen location. A failed check leaves the previous
location pointer in place. The action also makes a GUI restore-as-copy usable:
after restoring to a separate folder, the user can select that copy on the home
screen. The app never moves or deletes the selected data during attachment.

In a disposable GNOME 46 Wayland VM, ordinary Flatpak uninstall retained the
external personal VM, its guest boot files and an unrelated shared folder. The
app was then installed again from the private bundle, found in GNOME Activities,
and used through the installed GUI to select the retained external folder. The
portal granted the folder without a broad filesystem override. Closing and
reopening the app kept the choice. Launch booted the retained Omarchy desktop;
QEMU used the selected disk and matching guest kernel and initramfs. The guest
shut down cleanly. A desktop VM reboot preserved the portal grant and saved
location. [Home after desktop reboot](phase10g/reattach-after-vm-reboot.png)
shows the same 24 GiB disk and portal path.

This is one installation-lifecycle slice. It is not a complete Phase 10 or
release-candidate result.

## Source, bundle and guest identity

- Checkout: `/home/bts/Projects/try-omarchy-linux-core`, branch `linux-core`,
  starting HEAD `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`. Inherited
  uncommitted changes, disks, snapshots and evidence remain in place. No commit,
  push, PR, issue or publication occurred.
- Build source: `/workspace/try-omarchy-linux/phase10g-reattach-src` on devbox,
  mirrored at `/home/bts/.cache/try-omarchy-phase10g-source`. The
  [reconstructable delta](phase10g/source-delta.tar.gz) from the Phase 10E
  source changes six files, removes none, and covers a 737-file snapshot. Its
  [manifest](phase10g/source-delta.json) records per-file hashes and the parent
  source delta and bundle hashes. Starting Phase 10E bundle SHA256 is
  `0c8386ca78a5b1369f9269bc992a5d09c6bda0880c594adaee6fc895d785a05a`.
  A [reconstruction check](phase10g/reconstruction-check.txt) applied the delta
  to the parent snapshot and checked all six changed file hashes and the 737
  file count. The current worktree, local mirror and devbox source hashes of the
  final changed implementation file also matched.
- Final bundle: [private Phase 10G Flatpak](phase10g/phase10g-reattach-final.flatpak).
  SHA256 `224d68c03487ee9b1556483fa083050863bd372859ce8cc9684138f11e4b6aad`;
  installed GNOME OSTree commit
  `cf8266a2f1a8e3571750067885588f2177888d6d13fb14c3173f681d6fa7367d`.
  The final source-delta SHA256 is
  `7aa3837a98f6864105033619429bd4dd934361a384ce658fab16e3adcd54e7cf`.
  These are also in [final identities](phase10g/final-identities.txt). The
  [build log](phase10g/flatpak-build-final.log) is retained. The first live
  GUI pass used the earlier Phase 10G bundle,
  SHA256 `523d35abab96f120d25779cee29996690260d5a52ba70e1b95cd5db42b274193`.
  A narrow removable-location race fix prompted the final rebuild; final bundle
  checks are recorded separately below.
- Guest: retained private compatibility revision 39 and personal disk from
  Phase 10E. The inherited, previously checked `SHA256SUMS` hash is
  `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`;
  `guest-manifest.json` is
  `a572cdac779f871c38ee4e2048204e131ea34195443815a336256aa7904e04a4`;
  kernel `8ebc2c71271e000540f0a7545afd8e73504fcb724671f3d38c720b241842b901`;
  initramfs `3247b8993430da428b2d0a219bea2786ebff461cfb83255680c654095f37766a`;
  rootfs `313839548ae9ceec2905a5d5a9535aa13d7dca97faf4539c90811cfc00e5ce2a`.
  These hashes are inherited from the
  [Phase 10E report](LINUX-SOL-PHASE10E-FILE-PORTAL-2026-09-27.md), not
  recomputed in this run.
- Desktop isolation: devbox GNOME overlay
  `/workspace/try-omarchy-linux/phase10g-lifecycle/desktop-overlay.qcow2`
  started as a copy of the Phase 10F overlay, SHA256
  `2357984e3edf2984c6ac28d1bd24dea962865ec9d44bd76540c7ff079221108f`.
  The final overlay SHA256 after a clean desktop VM poweroff is
  `11084c0348b67082f9e8796906c995f5a1e9edacffc2fac7fda027e8edccb126`.
  Its QEMU PID file was removed. It uses the existing matrix backing image.
  Only the task's `phase10g-lifecycle` omabox and VNC tunnel displayed and
  drove it. No real desktop, session bus, audio, clipboard, camera or input
  device was used.

## Observed installed workflow

| Step | Evidence and result |
| --- | --- |
| Pre-removal | [Pre-uninstall record](phase10g/pre-uninstall.txt) has the original Flatpak commit, disk inode/size/mtime and guest artifacts. |
| Ordinary removal | `flatpak uninstall --user -y com.tryomarchy.TryOmarchy` removed the app. [Post-uninstall record](phase10g/post-uninstall.txt) shows the same disk inode, size and mtime, retained guest artifacts and retained unrelated external folder. This command did not request data deletion. |
| Reinstall and discoverability | `flatpak install --user -y --noninteractive` from the private bundle restored the desktop entry. [Activities search](phase10g/app-search.png) found Try Omarchy. [Post-reinstall record](phase10g/post-reinstall.txt) has the installed commit and unchanged disk identity. |
| Folder portal and attach | The installed [home](phase10g/home-initial.png) exposed Use existing data folder. The [folder chooser](phase10g/attach-data-select.png) selected the retained `matrix/data` folder. The [result](phase10g/attach-result.png) showed the granted `/run/user/1000/doc/92d05703/data` path. There was no broad Flatpak filesystem override. |
| Persistence and guest | [App reopen](phase10g/reattach-reopen.png) found the saved 24 GiB disk. [Launch state](phase10g/reattach-launch.png) and [guest desktop](phase10g/reattached-guest-desktop.png) show the retained VM running. The [shell log](phase10g/installed-shell-first-run.log) identifies the portal disk and guest boot artifacts, guest desktop readiness and clean poweroff. |
| Desktop reboot | The disposable outer GNOME VM rebooted; the portal path and saved VM remained visible on [app reopen](phase10g/reattach-after-vm-reboot.png). |
| Final safety rebuild | The final bundle was installed over the first test build, with [commit and disk identity](phase10g/final-install.txt). [Final home](phase10g/final-home-saved.png) still found the retained disk. [Final guest desktop](phase10g/final-guest-desktop.png) rendered normally. The [final shell log](phase10g/installed-shell-final.log) identifies the same portal disk and guest boot artifacts, desktop readiness and clean poweroff. [Final state](phase10g/final-state.txt) retains the pointer and disk inode. |

The disk inode stayed `64769:1121432` and size stayed `25769803776` bytes
through uninstall and reinstall. Its mtime changed only after the retained guest
boot wrote to the disk. This is retention evidence, not proof that every guest
file is intact; the actual guest desktop boot provides the behavioral check.

## Regression checks and remaining work

- `TestLinuxAttachExistingVMAndKeepPreviousOnFailure` covers absent and
  incomplete folders, preservation of the previous pointer, reattaching a
  restored copy, reselecting the default VM and preserving both disk files.
  Focused location and restore tests also passed. Linux `go vet ./...` and
  `git diff --check` passed. Tests ran with host display and session-bus
  variables removed; GUI observation stayed in omabox and the private VM.
  Saved [focused test](phase10g/focused-tests.txt), [vet](phase10g/vet.txt)
  and [diff check](phase10g/diff-check.txt) outputs document the final source.
- Windows vet, cross-build and test compilation passed on the local cached Go
  toolchain with display and session-bus variables removed. See the
  [cross-check log](phase10g/windows-cross-check-local.txt). The first attempt
  in a devbox Go container could not fetch `github.com/klauspost/compress`
  because its DNS lookup timed out; its [log](phase10g/windows-cross-check.txt)
  is retained. No Windows tests were run natively. The Flatpak build remained
  on devbox.
- The final attach path probes an existing folder without creating it, then
  rechecks all required VM files before recording a pointer. This protects
  against a removable folder vanishing between the first check and write probe.
- This run did not perform a fresh first install from an end-user distribution
  source, GNOME Software uninstall, explicit confirmed deletion of disposable
  app-owned data, removable drive removal/regrant, low-disk behavior or
  corrupt-settings recovery. App and guest updates, rollback, reset and move
  remain Phase 10 gates. Phase 9 everyday parity and the Phase 11 exact
  candidate matrix, full QEMU-compatible Linux suite, Windows CI and physical
  acceptance also remain open. Publication requires a separate instruction.

The next concrete batch is explicit disposable deletion and recoverable
location failure, followed by normal fresh install and update/rollback
rehearsal. Artifact hashes for this run are in
[phase10g/artifact-sha256.txt](phase10g/artifact-sha256.txt).
