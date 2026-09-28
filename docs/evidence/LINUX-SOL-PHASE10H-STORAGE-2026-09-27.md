# Phase 10H private storage recovery, 2026-09-27

## Result

The installed home screen now handles an unavailable saved data location. It
explains the failure, keeps About, folder reattachment and a new **Forget
unavailable location** action available, and disables launch, settings and
recovery until storage is accessible. Forget asks for confirmation and removes
only `data-location.json`. The selected VM and shared folders are untouched.
The action also works when the saved pointer is corrupt. It refuses to remove a
symlink in place of that pointer.

In a disposable GNOME 46 Wayland VM, the final private Flatpak passed a folder
disconnect, Keep and Forget confirmation, reconnection and portal reattachment.
It also recovered after the document portal export was revoked. A new grant
received a new document ID and the original personal VM booted. The same final
bundle passed Flatpak `--delete-data` uninstall, reinstall, GUI reattachment
and retained guest boot. These are observed software checks, not physical
removable-drive acceptance or a complete Phase 10 lifecycle result.

## Exact identity and recovery state

- Checkout: `/home/bts/Projects/try-omarchy-linux-core`, branch `linux-core`,
  starting HEAD `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`. All changes
  remain uncommitted and local. The existing dirty worktree, disks, source
  snapshots and evidence were preserved.
- Source parent: `/home/bts/.cache/try-omarchy-phase10g-source`. Phase 10H
  source: `/home/bts/.cache/try-omarchy-phase10h-source`, mirrored at
  `/workspace/try-omarchy-linux/phase10h-storage-src` on devbox. The
  [reconstructable delta](phase10h/source-delta.tar.gz), SHA256
  `f64f5af024a6cdfff011bc0b29d67d3a2a37afbc13f609f7d67af1bb2155bb0d`,
  changes five files in a 737-file source snapshot and removes none. The
  [manifest](phase10h/source-delta.json) records changed file hashes, starting
  HEAD and parent bundle/source identities. The
  [reconstruction check](phase10h/reconstruction-check.txt) and
  [devbox hash check](phase10h/devbox-source-check.txt) passed.
- Final [private Flatpak](phase10h/phase10h-storage-final.flatpak), SHA256
  `1ebd5fab35c0c8e9901f533062b6856faa0bf00c4b50de82ae2d893126a85649`.
  The installed OSTree commit was
  `0332b6a050e9547fe19c6f71b594b86eb455565ae155e1fb22e7237291ba2514`
  before and after the final delete-data rehearsal. The devbox bundle copy had
  the same hash. The [build log](phase10h/flatpak-build-final.log) is retained.
  The earlier intermediate bundle and build log remain in the evidence folder
  but are not this result's candidate.
- Guest: the retained private compatibility revision 39 personal VM from Phase
  10G. The kernel and initramfs hashes were recomputed before and after final
  delete-data: `8ebc2c71271e000540f0a7545afd8e73504fcb724671f3d38c720b241842b901`
  and `3247b8993430da428b2d0a219bea2786ebff461cfb83255680c654095f37766a`.
  The rootfs hash `313839548ae9ceec2905a5d5a9535aa13d7dca97faf4539c90811cfc00e5ce2a`,
  `SHA256SUMS` hash
  `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`
  and guest manifest hash
  `a572cdac779f871c38ee4e2048204e131ea34195443815a336256aa7904e04a4`
  are inherited Phase 10E/10G evidence, not recomputed here.
- Desktop: disposable devbox overlay
  `/workspace/try-omarchy-linux/phase10h-storage/desktop-overlay.qcow2` was
  copied from the stopped Phase 10G overlay, starting SHA256
  `11084c0348b67082f9e8796906c995f5a1e9edacffc2fac7fda027e8edccb126`.
  After clean desktop VM poweroff its SHA256 was
  `38228953bd0abffc8d1575a8f80ebf9519381444dbe6758714c9e8363dda43ae`.
  It uses the existing private matrix backing image. The outer QEMU process,
  task VNC tunnel and `phase10h-storage` omabox are stopped. Other workloads
  were left alone. See [final identities](phase10h/final-identities.txt).

## Observed installed workflows

| Check | Evidence and result |
| --- | --- |
| Baseline defect | The inherited Phase 10G [home](phase10h/baseline-missing-location.png) kept Launch available after the selected external folder disappeared. |
| Missing folder | The disposable `matrix/data` directory was renamed offline, preserving disk inode `64769:1121432` and 24 GiB size. The final [home](phase10h/final-missing-home.png) explained that saved storage was unavailable and offered reconnect, choose or forget. Launch, Settings and Backup/Recovery were insensitive. |
| Keep and Forget | The final [confirmation](phase10h/final-forget-confirm.png) named the saved portal path and said VM and shared files would stay. Keep retained the pointer in [state](phase10h/final-kept-pointer.txt). Forget removed the pointer; [home](phase10h/final-forgot-home.png) and [state](phase10h/final-forgot-state.txt) showed the disk and shared sentinel unchanged. |
| Reconnect and reattach | The folder was restored under its original name, with the same disk inode/size. The installed folder picker restored the pointer in [state](phase10h/final-reattached-state.txt), and the retained [guest desktop](phase10h/final-retained-guest-desktop2.png) rendered. The [shell log](phase10h/installed-shell.log) identifies the portal disk and guest boot pair and records clean poweroff. |
| Portal revocation | Removing the app permission entry alone left the already exported FUSE path readable. The [permission record](phase10h/revoked-grant-state.txt) and [home](phase10h/revoked-home.png) capture that limit. `flatpak document-unexport --doc-id 92d05703` then made sandbox `stat` fail in the [unexport record](phase10h/document-unexport.txt). On app reopen, the [home](phase10h/revoked-unexport-home.png) reported storage unavailable. The installed [portal picker](phase10h/revoked-portal-picker.png) reattached the same folder as document `bfae2c61`; [state](phase10h/revoked-reattached-state.txt) preserved disk inode/size and shared sentinel hash. The retained [guest desktop](phase10h/revoked-reattached-guest.png) booted again and was powered down through QMP. |
| Final delete-data | A disposable file was placed in the app's own data directory. [Before](phase10h/final-delete-data-before.txt) records its hash, the exact installed commit, external disk inode/size, shared sentinel and guest boot hashes. `flatpak uninstall --user --delete-data -y` removed the fixture and pointer. [After](phase10h/final-delete-data-after.txt) retains the external disk inode/size, shared sentinel and matching guest boot hashes. |
| Reinstall and boot | [Reinstall](phase10h/final-reinstall.txt) restored the same OSTree commit. The installed [fresh home](phase10h/final-reinstalled-fresh-home.png) held no saved VM. The [portal picker](phase10h/final-reinstall-picker.png) selected the external folder and [home](phase10h/final-reinstall-reattached.png) found the retained VM. GNOME prompted again for Remote Desktop clipboard consent in the [consent screenshot](phase10h/final-reinstall-consent-choice.png). The [retained desktop](phase10h/final-reinstall-retained-desktop.png) rendered. QMP [powerdown](phase10h/final-reinstall-powerdown.txt) and [final state](phase10h/final-reinstall-state.txt) show a clean guest exit, saved pointer and unchanged disk inode/size, sentinel and boot hashes. |

The disk mtime naturally changed during actual guest boots. Disk inode, size,
guest boot hashes, shared-folder sentinel and a successful retained desktop
boot are the preservation checks. The full 24 GiB disk was not hashed.

## Regression and limits

- `TestLinuxForgetUnavailableLocationKeepsVMAndSharedFiles` covers Keep,
  Forget, disconnected storage, unchanged VM/shared files, reconnect and
  reattachment. `TestLinuxForgetCorruptPointerButRejectSymlink` covers an
  unreadable pointer and refusal to remove a symlink. Targeted Linux tests,
  Linux vet and Windows vet/cross-build/test compilation passed with host
  display and session-bus variables removed. The [source checks](phase10h/source-checks.txt)
  record these results. Native Windows tests were not run.
- The Flatpak `--delete-data` command is an explicit package-manager action,
  not an in-app path-confirming delete flow. The test proves app-owned data
  removal and external data retention for this fixture. It does not prove
  deletion of a default VM disk, desktop Software removal, or user-facing
  confirmed deletion of disposable app-owned VM data.
- The offline folder was simulated by a rename in the disposable VM. Physical
  removable media, mid-write removal and real permissions remain deferred.
  Portal permission removal and document unexport have different immediate
  effects in this GNOME test. The product recovered after the latter.
- Normal fresh installation through the intended distribution route,
  low-space and corrupt-settings GUI recovery, updates/verification/rollback,
  reset/relocation, exact-candidate desktop matrix, full Linux suite,
  authorized Windows CI, physical acceptance and publication remain open.
  This checkpoint is not a release-ready candidate.

## Next actions

Finish the remaining Phase 10 lifecycle and update gates against disposable
private fixtures. In particular, add a path-confirming GUI action for deleting
an app-owned default VM without touching shared or external data, check desktop
Software uninstall, and rehearse private app/guest updates and rollback. Then
freeze an exact candidate and run Phase 11. Publication still requires a
separate instruction.
