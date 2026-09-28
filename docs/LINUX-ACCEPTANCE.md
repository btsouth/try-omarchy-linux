# Linux acceptance

The Linux repository and `linux-v0.1.0` guest are public. The app has not been
released. Current candidate changes and bundles remain unpublished.

First-release scope, confirmed 2026-09-28: core VM use, files, recovery and
desktop support. Camera capture, live audio switching, gestures, LAN/bridge,
USB, host-app launching and host authentication are documented limitations.
Physical acceptance is pending outside testers; Brandon has no separate Linux
test machine. The historical checkpoints below describe their own artifacts,
not the current candidate.

Use [Linux release gates](LINUX-RELEASE-GATES.md) for the complete end-user
lifecycle checklist and next work, alongside the checks in this document.

## Repeatable checks

Run `scripts/linux/check.sh /data/try-omarchy-checks` for Linux race tests,
Linux vet, Windows vet, and Windows cross-build/test compilation. Native Windows
tests run only in the repository's Windows CI job.

Build with `runtime-build/linux/build-flatpak.sh /data/try-omarchy-flatpak`.
The build container and downloaded sources are pinned. Module builds have no
network access; the builder downloads authenticated source archives first.
The output includes the bundle and its SHA256 digest. This is not a claim of
bit-for-bit reproducibility across different SDK revisions.

Install the candidate bundle in the test account, then run:

```sh
scripts/linux/smoke-flatpak.sh /data/try-omarchy-smoke \
  http://127.0.0.1:18090 TRUSTED_SHA256SUMS_DIGEST
```

The script creates an invisible omabox, blocks host session services and audio,
uses a new VM directory and temporary SSH key, checks guest service readiness,
saves a screenshot, and requests a clean guest shutdown. Inspect the screenshot;
SSH alone does not establish visual acceptance. The output directory is retained.
Use a fresh directory for each run. Do not run alongside another launcher.

`runtime-build/linux/ci/private-acceptance.yml` is a disabled workflow template
for a disposable runner with KVM and omabox. It deliberately lives outside
`.github/workflows`. Enabling or publishing it requires a separate decision.

## Product acceptance before publication

The current candidate has runtime/integration evidence, not feature-parity
sign-off. Complete the outstanding [product phases](LINUX-PLAN.md) and review
[Linux parity](LINUX-PARITY.md) before treating it as release-ready. Ordinary
use must work through the GUI with the intended Linux guest, without test-only
release arguments. Resolve serious known issues and have Brandon explicitly
accept any remaining parity gaps. Publication always needs his separate request.

## Phase 8A checkpoint

The pre-boot home and revised Settings form are local source changes. Focused
launcher tests cover direct-start precedence, idle-home read-only behavior,
Settings save/cancel preservation and carrying new-install preferences to a
selected storage folder. The exact GTK build, isolated screenshots and
remaining visual checks are recorded in the Phase 8 evidence report.

This checkpoint does not settle first-run guest readiness, normal Linux guest
selection, recovery, portal file grants, broader parity or physical acceptance.

## Phase 8B checkpoint

The Linux launcher now selects a pinned private Linux guest on ordinary launch
without release arguments. The default URL is local loopback and exists only for
private acceptance. The Windows release defaults remain separate. Guest patch
0101 reports desktop readiness after an Omarchy graphical session, monitor and
stable background/bar surfaces are present. Compatibility revision 39 updates
the service on existing private disks. Setup remains open through earlier QMP
and userspace-ready signals. The host reports a timeout or early VM exit instead
of silently closing setup. Local first-run and returning-boot checks reached the desktop in
omabox; see the exact artifact identities and limits in the [Phase 8B evidence](evidence/LINUX-SOL-PHASE8B-2026-09-26.md).
The readiness patch is now 0107 at compatibility revision 43, after the
rebase onto the shared guest series.

Before publication, replace the loopback fixture with an approved public Linux
artifact plan and independent checksum pin, then repeat fresh and returning
boots against that exact candidate. The Phase 8B checkpoint does not cover the
remaining product parity, recovery, external portal and physical gates below.

## Release gates

- Verify the exact bundle and guest checksums, not an earlier build.
- Boot a fresh disk and migrate a saved disk without losing personal files.
- Check setup cancellation, resume, folder grants after restart, account choice,
  close confirmation, and failure of the setup helper.
- Check clipboard text, PNGs and files in both directions, reconnection, transfer
  cancellation, and folder/drop access under the Flatpak sandbox.
- Check GNOME, KDE Wayland, an X11 desktop, and Omarchy. On GNOME Wayland,
  verify the Remote Desktop portal consent prompt, grant, two-way clipboard,
  denial, revocation and token restore on the installed candidate.
- Check scaling, keyboard overrides, tray and guest Settings entry, and audio
  device loss/recovery in isolated sessions.
- Run physical laptop, microphone, speakers, suspend/resume and real multi-monitor
  checks only when Brandon asks. These remain separate from automated acceptance.
- Confirm the main checkout is untouched and nothing was pushed or published.

## Outstanding targeted acceptance

Live external-file drops now have an installed GNOME passing path. The Phase 10E
Flatpak had no direct access to an external GNOME Files fixture. The Documents
FileTransfer portal granted the file for host-to-guest clipboard transfer;
installed GUI copy/paste and matching SHA256 passed in both directions without
a broad filesystem grant. Phase 10F corrected the original drag result:
native GNOME Files drops reached guest Downloads without a chooser because
GNOME supplied a portal grant. Fresh small and 64 MiB drops, a restarted app,
an explicit chooser grant and cancellation passed. Direct drops into guest
apps, transfer interruption, revoked document grants and reconnect still need
live acceptance. See the [Phase 10F correction](evidence/LINUX-SOL-PHASE10F-DROP-CORRECTION-2026-09-27.md),
[Phase 10E evidence](evidence/LINUX-SOL-PHASE10E-FILE-PORTAL-2026-09-27.md)
and [GNOME portal evidence](evidence/LINUX-SOL-GNOME-PORTAL-2026-09-27.md). The final QEMU
small-window sizing fix passed Intel, NVIDIA omabox and X11 checks; rerun GNOME
and KDE against the exact final bundle before release. Earlier matrix results
and artifact identities are in [integration evidence](evidence/LINUX-INTEGRATION-2026-09-26.md).

Phase 10G exercised package-manager uninstall without data deletion, retained
external VM and guest files, reinstall, GNOME folder-portal selection, saved
location on app reopen and a real retained guest boot. See the
[Phase 10G report](evidence/LINUX-SOL-PHASE10G-REATTACH-2026-09-27.md). This
does not cover desktop Software uninstall, fresh installation or update/recovery
acceptance. Phase 10H then passed installed-GUI missing-folder, Keep/Forget,
reconnect and document-unexport recovery. The exact final bundle passed
Flatpak `--delete-data`, reinstall, portal reattachment and retained guest boot;
the app-owned fixture and pointer were removed while the external disk, guest
boot artifacts and shared sentinel remained. See the
[Phase 10H report](evidence/LINUX-SOL-PHASE10H-STORAGE-2026-09-27.md).
Physical removable media and mid-write loss remain open. Removing a document permission entry
alone did not revoke the live FUSE export in the disposable GNOME VM;
document unexport did, and the installed GUI recovered.
Phase 10I then installed that exact private bundle in a new Ubuntu GNOME
account. Launcher discovery, cancel before storage selection, default personal
setup to a visible Omarchy desktop, clean shutdown and returning boot passed.
The second boot used the saved disk without another guest download. See the
[Phase 10I report](evidence/LINUX-SOL-PHASE10I-FIRST-INSTALL-2026-09-27.md).
The account was clean; the OS was an inherited configured VM, and installation
used Flatpak CLI from a private bundle. The intended graphical distribution
route and public artifact service remain unverified.
Phase 10J then upgraded the installed private Flatpak, found the saved default
disk, and passed Keep and path-confirmed Delete. The app-owned VM and guest
artifacts disappeared, while the selected shared-folder sentinel and saved
Settings survived. Relaunch after deletion offered storage selection. Private
Flatpak rollback to Phase 10H and re-upgrade to Phase 10J kept the shared
sentinel, but both ran after deletion and did not establish retained-VM update
safety. See the [Phase 10J report](evidence/LINUX-SOL-PHASE10J-DELETE-2026-09-27.md).
Phase 10K exercised Phase 10H to 10J upgrade, rollback and re-upgrade in a
separate disposable child with the personal VM retained. Each package found
the same disk and matched guest boot pair; the actual guest Documents file
kept its SHA256 on each visible boot. One rolled-back automatic GPU boot showed
a black guest surface despite desktop-ready. A clean QMP poweroff and the
installed Software rendering setting recovered a visible desktop and file;
automatic retry then rendered. This is an open graphics stability finding for
the exact-candidate matrix. See the
[Phase 10K report](evidence/LINUX-SOL-PHASE10K-RETAINED-UPDATE-2026-09-27.md).
Windows vet, cross-build and test compilation passed for the Phase 10J source;
native Windows CI remains open. Phase 10N and the later exact Phase 10P source
passed the full Linux race suite, including both saved-session tests, with
packaged QEMU 11.1.1 in the pinned Flatpak SDK. Host QEMU 8.2.2 still cannot
run those two tests because it rejects `exit-on-error`.

Phase 10N then passed installed GUI backup of a portal-reattached personal VM,
restore into a second independent copy, reattachment and a visible guest boot
with its original Documents file. The restored disk matched the archive
manifest SHA256 before boot, and the kernel/initramfs pair matched. The
installed GUI refused a low-space backup before output. A cancelled restore
removed its staging copy but exposed a helper that never left “Cancelling
setup...”. Phase 10O returned to home but left Close disabled. Phase 10P passed
Cancel, explicit preservation message and Close on the exact installed bundle;
all three prior VM disk inodes and an empty restore destination were observed.
A corrupt archive showed a visible error and no output. See the
[Phase 10N report](evidence/LINUX-SOL-HANDOFF-PHASE10N-2026-09-27.md),
[Phase 10O interim finding](evidence/LINUX-SOL-HANDOFF-PHASE10O-2026-09-27.md)
and [Phase 10P final handoff](evidence/LINUX-SOL-HANDOFF-PHASE10P-2026-09-27.md).
This does not close GUI reset/move, guest updates, the exact desktop matrix,
native Windows CI or owner-run physical acceptance.

## Current limits

The host keyboard layout follows exposed `XKB_DEFAULT_LAYOUT` and
`XKB_DEFAULT_VARIANT` values, or `-keyboard`. It does not monitor desktop layout
switches. `-keyboard keep` preserves the guest choice. `-scale keep` preserves
the prior guest scale policy; numeric scale overrides are available.

Audio defaults to PipeWire, with silent fallback if startup fails. Named routes
and microphone disable are exposed in Linux Settings. Physical playback remains
unverified. LAN forwarding, camera capture, packaged battery behavior and the
remaining Linux recovery lifecycle remain Phase 9 and 10 work. Their physical
acceptance is separate from isolated functional checks.

## Interrupted restore housekeeping

Cancelling normally removes this operation's staging files. A power loss or
forced process termination can leave an unpublished `.try-omarchy-restore-*`
folder in the selected destination. The current VM is retained. Verify that the
current VM and backup work before removing an identified incomplete staging
folder; do not remove a completed restored VM or unrelated files.
