# Phase 10P private handoff for Astra

Observed 2026-09-27. This is the current local correction bundle and a review
checkpoint, not a public release candidate. All work remained on `linux-core`
with uncommitted work preserved. No commit, push, PR, issue, site change,
distribution upload or physical desktop test occurred.

## Exact identities

| Item | Identity |
| --- | --- |
| Starting HEAD | `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae` |
| Phase 10P source | `/home/bts/.cache/try-omarchy-phase10p-source`, 746 files, byte-matched to devbox `/workspace/try-omarchy-linux/phase10l-graphics-src` |
| [Phase 10P delta](phase10p/source-delta.tar.gz) | SHA256 `7a70949a81c87937f5ce73677b40e41d97d20a1da8af2e89b9fefc8cbd961473`; one changed file from the Phase 10O source, plus [manifest](phase10p/source-delta.json) and [reconstruction check](phase10p/reconstruction-check.txt) |
| [Phase 10P Flatpak](phase10p/phase10p-cancel-final.flatpak) | SHA256 `1c544e8546631ac8eca8abea583ca60a471ed443e750f88e1ac5341741aa3367`; installed OSTree `65c929d7997843a805d0dbac9b8d0b89d6eee95455232e3249c0e6502e32eb2d` |
| Guest | Compatibility revision 39; trusted `SHA256SUMS` SHA256 `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`; kernel `8ebc2c71271e000540f0a7545afd8e73504fcb724671f3d38c720b241842b901`; initramfs `3247b8993430da428b2d0a219bea2786ebff461cfb83255680c654095f37766a` |
| Stopped outer desktop VM overlay | `/workspace/try-omarchy-linux/phase10m-capacity/desktop-overlay.qcow2`, SHA256 `3a7f1d962d1d6fd1ebe1dfef635311dc781dc0286980e7598a2ff68645d38fcb`, 160 GiB virtual, 26.8 GiB allocated, [backing chain](phase10p/overlay-chain.txt) through stopped Phase 10L and 10K overlays |

Reconstruct Phase 10P from the preserved Phase 10J source snapshot by applying
the Phase 10N, 10O and 10P source deltas in order. Their manifests record
parent hashes. The Phase 10O bundle was an interim result and is intentionally
preserved as [Phase 10O evidence](LINUX-SOL-HANDOFF-PHASE10O-2026-09-27.md).
The stopped Phase 10K parent overlay SHA256 is
`aeb34bf9709c64a58a1b3b96d809180955c5a8716e3dc9603598bf9ac0817fc8`.
Keep the overlay chain intact; never edit a backing image while its child is in
use. The private GNOME account, backup archives, original personal VM and two
restored copies remain inside that stopped child chain.

## End-user workflows and corrections

[Phase 10N](LINUX-SOL-HANDOFF-PHASE10N-2026-09-27.md) records the installed
GUI backup from a portal-reattached personal VM, an archive with no shared
folder entries, a second restore-as-copy, preboot disk SHA256 matching the
archive manifest, matched kernel/initramfs, visible Omarchy desktop, and the
same guest Documents file. The installed GUI refused a backup on a 24 GiB
low-space ext4 fixture before writing output. Portal mount symlink validation
and unsupported-FUSE-`flock` fallback were corrected to make the external
reattached backup actually pass. A synthetic virgl stderr error produced one
visible nonfatal graphics warning; the intermittent real black surface remains
unresolved. Those Phase 10N backup and boot observations were made on its exact
installed bundle, not repeated end to end on Phase 10P.

A real installed restore cancellation on Phase 10N removed staging and kept
the original disks but left the GTK helper on “Cancelling setup...”. Phase 10O
accepted the launcher's completed home state; the installed UI returned home,
allowed Backup and recovery, and surfaced an invalid-ZIP error without output.
Further inspection found its Close button remained disabled. The Phase 10P
source re-enables the button on the next accepted state. In the exact installed
Phase 10P GUI, restore was started from the same archive and cancelled at 5%.
The home showed “Operation cancelled. The original VM was kept.” with an
enabled Close button: [progress](phase10p/progress-before-cancel.png),
[home](phase10p/cancel-home-close-enabled.png). Clicking Close actually exited:
[desktop](phase10p/close-after-cancel.png). The cancelled destination had no
children. The original and both restored 24 GiB disks retained inodes 1123315,
4980753 and 9699343, and the guest boot pair remained matched. The
[installed check](phase10p/installed-candidate.txt) records the final OSTree,
inodes and boot hashes. The final Phase 10P test did not repeat invalid-ZIP
selection; Phase 10O did, and Phase 10P changed only the button reset.

The home screen still displays `/run/user/1001/doc/...` for a portal-selected
data folder. That is an opaque access path and a remaining product text issue.

## Checks on the exact Phase 10P source

- [GTK helper test](phase10p/ui-tests.log): passed with `gtk_4_18,adw_1_7`
  tags in the pinned SDK. Log SHA256
  `b84999c9055acce6d538ffb70a69a0cb804bf291ee2893b86381a5ab15057b18`.
- [Full Linux race suite](phase10p/compatible-suite.log): passed with packaged
  QEMU 11.1.1 and Go 1.27.1, including saved-session tests. App tests took
  45.465s and sign-update tests 1.016s. Log SHA256
  `885752e79a3145bb2ff4422e4cd9367b0f87637808f90c3b2fafe7080d9a2060`.
  The container printed a D-Bus teardown warning after successful tests and
  exited 0. Host QEMU 8.2.2 cannot run the two saved-session tests because it
  rejects `exit-on-error`; Phase 10N retained that failed host log separately.
- [Linux vet, Windows vet, Windows cross-build and Windows test compilation](phase10p/cross-checks.log):
  passed. Native Windows tests were not run locally and remain a CI gate. No
  push was made to trigger CI.
- Source snapshot and devbox build input matched under `rsync -nrc --delete`;
  extracting the delta over its parent also matched the snapshot. The pinned
  [build log](phase10p/build.log) records the Flatpak export commit.

## Installation, uninstall, update and recovery scope

The earlier Phase 10I clean account opened the private Flatpak from GNOME
search, completed personal setup and reached a visible desktop. Phase 10G/10H
removed and reinstalled the package, retained an external VM, reattached it
through the portal, and handled missing/revoked storage. Phase 10J confirmed
deletion of a disposable app-owned default VM while keeping its selected shared
folder. Phase 10K upgraded, rolled back and re-upgraded a retained personal VM
without changing its disk inode, guest file or boot pair. Phase 10N added the
installed backup/restore copy and low-space path; Phase 10P added verified
restore cancellation and Close. These are separate observed fixtures, and no
single exact Phase 10P run covered the whole installation lifecycle.

The installed route was a private Flatpak CLI install on a configured OS, not
the intended graphical distribution route. Guest artifact upgrade, signature
verification failure, interrupted apply, rollback and recovery are still
unexercised. The Linux release URL remains a local loopback fixture; public
artifact hosting and an independent checksum pin need an owner decision before
publication. GUI reset/relocation and confirmed deletion under failure are
still open. A short physical checklist is in the release gates, but no physical
test was authorized or run.

## Known defects and parity differences

- Intermittent black virgl surface observed on a rolled-back automatic GPU
  boot in Phase 10K. Software rendering recovered; the synthetic warning check
  does not establish that the actual failure is fixed.
- Portal backup safety fallback should receive adversarial review: unsupported
  lock detection, private QMP socket, QEMU `/proc` descriptor scan, unreadable
  descriptors, races and orphan processes. Do not treat the installed success
  path alone as proof of every concurrency case.
- Opaque document-portal storage path is shown on home after reattachment.
- GUI reset/move, guest update/rollback, direct drops into guest apps, more
  transfer interruption and revocation cases, camera, broader battery/device
  acceptance, LAN/live forwarding, USB, gestures, host-app launching and host
  authentication still differ from Windows or Mac or lack a decided Linux scope.
- No exact Phase 10P Omarchy, GNOME, KDE Wayland and X11 matrix, sustained-use
  soak, available Intel/NVIDIA pass, native Windows CI or physical acceptance.

## Next concrete work

1. Ask Astra to independently falsify the Phase 10P result, especially portal
   backup safety, cancellation and data preservation. Use the audit prompt
   below. Correct any P0/P1 finding before more release work.
2. Complete GUI reset/relocation and guest update, verification, interrupted
   apply, rollback and recovery using private fixtures. Repeat the intended
   graphical install and uninstall route with default, custom and removable
   data locations.
3. Freeze a later exact bundle, source and guest pair. Run the full desktop,
   hardware-available and sustained lifecycle matrix, plus Windows CI when an
   authorized branch push exists. Keep physical acceptance and publication as
   separate owner gates.

## Fresh-eyed Astra prompt

> Independently audit the local Try Omarchy Linux Phase 10P candidate. Start
> with `docs/evidence/LINUX-SOL-HANDOFF-PHASE10P-2026-09-27.md`, then inspect
> the Phase 10N and 10O reports, source deltas and exact bundle hashes. Try to
> falsify the reported end-user results rather than accepting the screenshots.
> Focus on P0/P1 defects, new regressions, data loss, false success, portal
> permission boundaries, unsupported `flock` fallback, QEMU orphan/race checks,
> restore cancellation and Close recovery, low space and corrupt archives,
> shared-folder exclusion, guest boot-pair integrity and graphics warning
> behavior. Reproduce through the installed GUI and actual guest in a fresh
> disposable devbox desktop VM; use omabox for all GUI control. Run exact-source
> focused regressions and the compatible-QEMU suite if a finding requires it.
> Keep work local and private. Do not touch the real desktop, session bus,
> audio, clipboard, camera or input devices. Do not commit, push, open PRs,
> upload, publish or run physical hardware checks. Preserve existing overlays,
> snapshots and evidence. Report each P0/P1 with reproduction steps and
> observed versus expected behavior, distinguish your observations from Sol's
> inherited evidence, list untested cases and give an explicit accept/reject
> verdict for this private checkpoint. Do not infer public release readiness
> from a passing audit.
