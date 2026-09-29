# Linux release gates and next work

Updated 2026-09-28 after the Linux code moved to its own public repository.

Scope confirmed 2026-09-28: the first Linux release requires core VM use, files,
recovery and desktop support. Camera capture, live audio switching, gestures,
LAN/bridge networking, USB, host-app launching and host authentication are
documented first-release limits, not implementation gates. Brandon only has an
Omarchy system available; physical acceptance remains for outside testers. Do
not count nested desktop results as hardware acceptance or request the deferred
owner hardware pass again. Release and Flathub publication remain separate
decisions.

Brandon wants a public release only after everyday features, branding, install,
uninstall, updates and recovery are solid, with near parity to Windows and Mac.
This document makes that the acceptance target for phases 9 through 11. It does
not authorize a release, Flathub submission or physical testing.

The existing [plan](LINUX-PLAN.md), [parity inventory](LINUX-PARITY.md) and
[acceptance instructions](LINUX-ACCEPTANCE.md) remain applicable. A feature is
complete when its end-user workflow passes, including failure and restart
behavior. A backend, CLI flag or screenshot alone does not establish that.

## Current acceptance evidence

The [2026-09-28 continuation report](evidence/LINUX-RELEASE-CLOSURE-2026-09-28.md)
records the GNOME 51 candidate, review corrections, final automated checks and
installed desktop results. The phase notes below are historical evidence; their
open-item lists are not a claim that later fixes are missing.

## Immediate review closure

1. Fix the microphone persistence retry finding in
   [Astra's review](evidence/LINUX-ASTRA-REVIEW-PHASE9-2026-09-27.md).
   Preserve unrelated device preferences and cover failed Save followed by retry.
2. Recheck the late-desktop timeout dismissal with the final packaged helper and
   a real delayed personal-account setup. The current fix has deterministic
   coverage; the successful late-desktop dismissal still needs live evidence.
3. Retain a new source delta, bundle hash, guest identity and focused results.
   Do not rerun the entire historical matrix for this small correction.

## Phase 9B: everyday controls

Implement and accept in small, reviewable batches:

- Audio output/input choices, microphone access, device loss and recovery.
  Prove disabled input is disabled through the supported audio paths. Use private
  audio services for development; real microphones and speakers are a later gate.
- Display preferences, fullscreen entry and exit, host scale behavior, keyboard
  layout selection and changes. Keep an obvious way to release keyboard capture
  and return to the host. Preserve intentional guest customizations.
- GUI resource, disk-capacity, networking/SSH and startup controls. Explain
  restart requirements and validate conflicts and invalid values before saving.
  Increasing disk capacity must preserve files and filesystem usability.
- Linux camera and host battery integration, with clear unavailable/denied-device
  behavior. Synthetic tests do not establish real hardware acceptance.

For each batch: use the GUI, save, close and reopen, launch the VM, verify the
actual guest behavior, then exercise the important error and recovery paths.
Verify changes while a VM is running cannot claim to apply before they do.

## Phase 9C: clipboard, files and desktop parity

- Complete actual external file-drop permission acquisition and FileTransfer
  portal grant round trips in the sandbox. Check cancellation, restart, revoked
  permission, non-ASCII names, large files and visible transfer failures.
- Verify text, image and file clipboard workflows in both directions, including
  reconnect and avoiding clipboard loops, on each supported desktop.
- Resolve the GNOME clipboard experience or present Brandon with a concrete
  supported workflow and an explicit gap decision. Do not silently treat a shared
  folder as clipboard parity.
- Update the Windows/Mac comparison using dated release references, separating
  released behavior from newer source features. Account for gestures, USB,
  host-app launching, LAN networking and authentication. Implement comparable
  everyday outcomes; document any platform-specific deferrals for Brandon to
  accept. Do not expand scope just to copy an API that Linux does not have.
- Verify every essential action remains accessible on desktops without a tray.

GNOME Wayland decision, 2026-09-27: Brandon chose consented Remote Desktop
portal synchronization. The installed GNOME 46 private bundle passed the
permission prompt, token reuse, two-way text/PNG/file clipboard and visible
session revocation. GNOME requires pointer permission in the dialog to enable
Share; the app sends no remote input. The initial file fixture used a test-only
Flatpak filesystem grant. Phase 10E later passed external-file GUI copy/paste
in both directions through the Documents FileTransfer portal without a broad
grant. Phase 10F corrected the native GNOME Files drop finding: small and
64 MiB drops reached guest Downloads; an explicit portal chooser grant and
cancellation also passed. Direct drops into guest apps, interruption, revoked
grants, a real GNOME Remote Desktop denied prompt and broader desktop coverage
remain open. See the [GNOME portal report](evidence/LINUX-SOL-GNOME-PORTAL-2026-09-27.md),
[Phase 10E report](evidence/LINUX-SOL-PHASE10E-FILE-PORTAL-2026-09-27.md) and
[Phase 10F correction](evidence/LINUX-SOL-PHASE10F-DROP-CORRECTION-2026-09-27.md).

## Phase 10A: installation, branding and removal

Treat the normal user journey as one acceptance test:

1. On a clean supported desktop VM/test account, install using the intended
   end-user Flatpak route, find the app in the launcher and open it without a
   terminal, development tools, diagnostic flags or a developer release server.
   A private distribution rehearsal may stand in for the eventual public service.
2. Check KVM/permission and resource guidance, download size and disk-use wording,
   default/custom storage, trial/personal setup, cancellation/resume and the first
   usable desktop. Reinstall must not unexpectedly recreate or overwrite a VM.
3. Review app name, icon, window/dock grouping, version, About/help, metadata,
   screenshots and installation instructions as one consistent product. Keep the
   existing Omarchy identity. Remove test paths and engineering terminology from
   normal user-facing guidance where they do not help the user.
4. Check small windows, fractional scale, light/dark themes, keyboard-only use,
   focus visibility, accessible labels and readable error/action text. Inspect
   the installed package, not just source or screenshots from a prior bundle.
5. Uninstall the application through the intended desktop/package-manager route.
   Document and test what happens to the VM, preferences and downloads. The
   recommended ordinary removal should retain personal VM data; explicit data
   deletion needs a clear confirmation showing the affected location.
6. Reinstall and reattach the retained VM, including custom and removable storage.
   Separately test explicit deletion of disposable app-owned data. Never delete
   shared-folder contents or assume Flatpak's data deletion reaches an external
   VM directory. Check stale launchers, running processes and obsolete grants.

The Flatpak app removal and VM-data deletion behaviors must agree with the UI
and documentation. Do all destructive acceptance against disposable fixtures.

Phase 10G added a home action to attach an existing VM through the folder
portal. In a disposable GNOME VM, ordinary Flatpak removal retained the
personal disk, guest artifacts and external shared folder. Reinstall exposed
the launcher, the portal granted the retained external VM, and the app saved
that choice, reopened it and booted the guest. See the
[Phase 10G report](evidence/LINUX-SOL-PHASE10G-REATTACH-2026-09-27.md).
This covers one custom location through the package manager. Fresh first-run
installation, removable-drive revocation, explicit confirmed deletion and
desktop Software removal remain open.

Phase 10H added an installed home recovery choice for unavailable saved
storage. In a disposable GNOME Wayland VM, the exact final Flatpak passed
disconnect/reconnect, confirmed Forget without VM deletion, document portal
unexport and regrant, retained guest boot, `--delete-data` uninstall and
reinstall. The external VM, shared sentinel and guest boot pair survived.
The [Phase 10H report](evidence/LINUX-SOL-PHASE10H-STORAGE-2026-09-27.md)
records exact hashes and the distinction between removing a permission entry
and unexporting the document. In-app path-confirming default-VM deletion,
desktop Software removal and physical removable media are still open.

Phase 10I installed that same bundle in a new account on a disposable Ubuntu
GNOME VM. Launcher discovery, default-location personal setup, a visible first
Omarchy desktop, clean poweroff and a returning desktop passed. Cancel before
location selection left no VM disk. The second start used the saved 24 GiB disk
without downloading guest files again. See the [Phase 10I report](evidence/LINUX-SOL-PHASE10I-FIRST-INSTALL-2026-09-27.md).
This was a private Flatpak CLI install in a clean account on a configured OS,
not the intended graphical distribution route or a clean OS install. That
route, trial mode, failure and resume paths, and the other Phase 10A checks
remain open.

Phase 10J added a path-confirming Delete action for the app-owned default VM.
On the installed private bundle, Keep retained the personal disk; confirmed
Delete removed its VM and guest artifacts while retaining Settings and a
portal-selected shared folder. The next launch offered storage selection and
Cancel did not recreate a disk. A private Flatpak rollback and re-upgrade
after deletion retained the shared sentinel. See the
[Phase 10J report](evidence/LINUX-SOL-PHASE10J-DELETE-2026-09-27.md).
Deletion during a failure and graphical Software removal remain open.

## Phase 10B: updates and recovery

- Define the Flatpak update channel and the Linux guest artifact/version policy.
  Replace the loopback default before release. Prepare privately the independent
  trust/checksum pin and hosted artifact layout, then rehearse the same download
  and verification behavior intended for public use.
- Exercise app and guest upgrades with a personal account and sentinel files.
  Prove interrupted downloads, failed verification, interrupted apply and rollback
  leave a usable VM with matched kernel, initramfs and disk state. Never lose
  private files or silently boot a mismatched combination.
- Provide GUI backup/restore, confirmed reset, storage relocation and diagnostics.
  Exercise low disk space, corrupt settings, unavailable/revoked storage and
  failure during each operation. Preserve the recoverable original until the
  replacement is verified. Redact diagnostics before users share them.
- Rehearse upgrade, rollback, uninstall and reinstall across two private versions.
  Include default, custom and removable data locations and a VM already running.

Phase 10K replaced the installed app from Phase 10H to Phase 10J, rolled back,
and re-upgraded with the same default-location personal VM retained. Both
versions booted the saved guest, and its Documents sentinel, disk inode and
installed kernel/initramfs hashes survived. One rolled-back automatic GPU boot
displayed black despite a desktop-ready log; installed GUI Software rendering
recovered, and a later automatic retry rendered. See the
[Phase 10K report](evidence/LINUX-SOL-PHASE10K-RETAINED-UPDATE-2026-09-27.md).
App package replacement is covered for this private pair. Guest update,
interruption, verification failure, running-VM update, other storage locations
and graphics stability remain open.

Phase 10N exercised the installed GUI against the retained personal VM. Backup
and restore-as-copy produced an independently bootable 24 GiB disk with the
same SHA256 as the archive manifest, a matched kernel/initramfs pair and the
same guest Documents file. A backup from a portal-reattached restored VM first
exposed the Flatpak document mount symlink and unsupported `flock`; the narrow
portal handling correction then passed in the final installed bundle. A 24 GiB
low-space fixture refused backup before writing output. See the
[Phase 10N handoff](evidence/LINUX-SOL-HANDOFF-PHASE10N-2026-09-27.md).
Restore cancellation found a separate stuck helper window. Phase 10O corrects
the stuck home state but left Close disabled. Phase 10P re-enables it after
cancellation; the exact installed bundle passed Cancel, preservation message,
then Close. See the [Phase 10O interim finding](evidence/LINUX-SOL-HANDOFF-PHASE10O-2026-09-27.md)
and [Phase 10P handoff](evidence/LINUX-SOL-HANDOFF-PHASE10P-2026-09-27.md).
GUI reset/move, guest updates, interrupted app replacement and other storage
types remain open.

The Phase 10P audit found no P0/P1. Phase 10Q corrects its two P2 findings:
backup now rejects a disk held by another QEMU through its image lock and
refuses to publish if the disk changed while it was read, and a graphics
warning is no longer lost behind another error window. The installed bundle
refused a backup while a host process modified the disk, still backed up the
same portal VM cleanly, and showed the queued graphics warning. See the
[Phase 10Q report](evidence/LINUX-PHASE10Q-BACKUP-SAFETY-2026-09-27.md).

## Phase 11: exact candidate and release decision

Freeze the bundle, guest artifacts, source snapshot and instructions by hash.
Against that candidate, complete the supported Omarchy, GNOME, KDE Wayland and
X11 matrix, with Intel and NVIDIA coverage already available. Include repeated
launch/shutdown/reboot, sustained everyday use, transfers, device recovery,
permission failures, resource pressure and the complete installation lifecycle.

Run the full Linux suite with a compatible QEMU version. Phase 10N and the
latest Phase 10P source passed the full race suite, including both saved-session
tests, with packaged QEMU 11.1.1 inside the pinned Flatpak SDK. The two failures on host QEMU 8.2.2 remain
version incompatibilities, not candidate regressions. Repeat this gate for
the final frozen source after any further edits.
Keep Windows cross-build, vet and test compilation green. Native Windows tests
run only in the repository's Windows CI job. If shared/Windows code changes,
record that CI requirement as pending until an authorized CI run covers the
candidate; local-only work does not authorize a push to obtain it.

Physical acceptance is assigned to outside testers, using the
[hardware checklist](LINUX-HARDWARE-TESTING.md). Real GPU, audio/microphone,
battery, suspend/resume and monitor changes remain unverified. Camera and
gestures are accepted first-release limits. Do not ask Brandon to provide
hardware he does not have or run these checks on his real desktop.

Release requires no known serious defect, reliable core everyday workflows,
passed lifecycle/branding gates, and Brandon's explicit acceptance of any
remaining limitations. Record the exact candidate and unresolved items. Passing
acceptance still does not authorize pushing, opening a public PR/issue, changing
the public site, publishing a repository or uploading a release.

## Repository and distribution

Current distribution decision: use a clean GitHub README and GitHub Releases
as the public entry point. A separate website and tryomarchy.com changes are
deferred. The existing signed Flatpak repository still requires static hosting
for automatic updates; GitHub Pages is package infrastructure, not a required
public landing page. See [release instructions](LINUX-RELEASING.md).


Decided 2026-09-28. The Linux app lives in the public repository
[btsouth/try-omarchy-linux](https://github.com/btsouth/try-omarchy-linux),
which keeps the Windows repository's history so shared launcher fixes merge
from `omacom/try-omarchy-windows`. It may move to `omacom` later. Public
releases go to Flathub as `com.tryomarchy.TryOmarchy`, verified through
tryomarchy.com, with the Linux guest image on this repository's releases.
Tag Linux releases `linux-vX.Y.Z`.

The branch is rebased onto Windows v0.6.0. The Linux guest patches are now
0103 through 0107 at compatibility revision 43, on top of the shared series.
QEMU aborts when a reconnecting socket chardev cannot connect at startup, so
the Linux launcher leaves out the Windows Hello authentication port. The
rebased Flatpak booted the retained personal VM to a visible desktop in the
disposable GNOME VM. CI passes, including native Windows tests.

Before the first release:

1. Done 2026-09-28: the guest is published as the `linux-v0.1.0` pre-release,
   and a fresh install with no flags downloaded it from GitHub and reached the
   desktop. See the [v0.1.0 report](evidence/LINUX-V010-CANDIDATE-2026-09-28.md).
2. Done 2026-09-28: the black GPU desktop came from QEMU rejecting the
   wallpaper texture's backing on fragmented guest memory. The Linux QEMU now
   accepts larger backing lists; an affected disk went from 5 of 7 black boots
   to 8 of 8 good ones. Confirm on real GPUs during the hardware pass.
3. Done locally 2026-09-28: metainfo includes release information; GNOME 51
   builds and passes the full Linux race suite, GTK tests, Windows cross-checks
   and manifest/AppStream validation. See the continuation report for the
   exact source and bundle identities.
4. Finish the remaining Phase 10B and Phase 11 items above.

## Next work

Continue in bounded batches through the order above. Record what changed,
exact artifacts, automated checks, observed GUI and guest behavior, and
untested cases for each batch. Use devbox for heavy work and omabox or the
disposable desktop VM for GUI checks. Physical testing, releases and the
Flathub submission need Brandon's approval.

## Preview publication authorization

On 2026-09-28 the owner authorized committing and publishing as appropriate.
The tested app bundle may be published as a GitHub prerelease for outside
testing. Use a separate `linux-app-vX.Y.Z-preview.N` tag to distinguish the app
from the existing guest release. Hardware acceptance and Flathub submission
remain open; this preview does not mark the stable release gates complete.
