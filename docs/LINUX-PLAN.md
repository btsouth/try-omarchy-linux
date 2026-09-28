# Linux implementation plan

Public repository, no releases yet. Releases need Brandon's approval.
The Windows launcher must continue to cross-build and pass vet. Native Windows
tests run only in CI. GUI tests use a local omabox or the isolated devbox
desktop. Physical laptop and real desktop-session checks remain deferred.

The [release gates and next work](LINUX-RELEASE-GATES.md) make everyday parity,
branding, installation, uninstall/data retention, updates and recovery explicit
acceptance requirements. Follow that order for the next reviewable batches.

## Phase 3: first run and lifecycle

Finish the GTK setup window, persistent storage location picker, trial or
personal account choice, shutdown confirmation, and expiring guest visibility
leases. Keep interrupted installations and existing account choices intact.

Acceptance: packaged first install and resume on Intel, cancellation without
data loss, close/dismiss/helper-failure behavior, six minutes without guest idle
lock while visible, and restored idle behavior after lease expiry. Package the
guest integration as the next numbered patch with compatibility migration.

## Phase 4: clipboard and files

Reuse the existing bounded clipboard and streaming file-transfer protocol.
Provide a Linux backend for text, PNG, and file selections on supported
Wayland desktops, with X11 support where available. GNOME Wayland must use a
consented Remote Desktop portal session for automatic clipboard access.
Connect native file drops and add an explicit portal-selected shared folder.
Do not grant broad home-directory access to the Flatpak.

Acceptance: Unicode text and PNG round trips, no clipboard echo, bounded
malformed input, reconnect, file cancellation and path validation, portal
sharing and restart, and no contact with the real desktop clipboard.

## Phase 5: desktop integration

Follow host scale and keyboard layout where the desktop exposes them, retain
explicit overrides, and preserve guest customizations. Add audio selection
and fallback behavior, then tray controls for the running VM and settings.
Use private audio services for automated checks; physical playback remains
an explicitly deferred acceptance check.

Acceptance: scale and layout changes in isolated desktops, reconnect and
unsupported-desktop fallbacks, private audio device loss/recovery, and tray
shutdown/settings behavior. Preserve Windows defaults and tests.

## Phase 6: guest and desktop matrix

Build a guest image containing the NVIDIA presentation and host visibility
patches. Test the exact Flatpak and guest artifacts on Intel and in GNOME,
KDE, and X11 desktop VMs. Check fresh and existing disks. Record artifact
hashes, desktop versions, failures, and physical checks still outstanding.

## Phase 7: repeatable private acceptance

Add offline Flatpak build and KVM smoke-test automation, preserve diagnostic
artifacts, and prepare a private release checklist. Keep publishing and
physical laptop acceptance separate. No workflow is enabled remotely during
this work.

## Phase 8: launcher experience and branding

Phase 8A is a tested private checkpoint. The source now has a pre-boot home with Launch,
Settings, version/help and saved disk location/status. Closing the idle home
exits without booting. Explicit CLI launches remain direct; `-launcher` opens
the home when a script needs it. The Settings form has automatic or bounded
manual memory and CPU allocation, rendering choices, shared-folder selection,
and a resource-defaults action. It explains that running-VM settings require
a new QEMU launch. A new install can save preferences before choosing a storage
location and carry them to that location. Isolated GTK visual acceptance and
exact-bundle runtime checks are recorded separately in the Phase 8 evidence.

The remainder of this phase needs a connected first-run and guest-ready flow,
clear preflight and transfer errors, accurate metadata, and visual/accessibility
review across desktops. Keep native Linux controls and the existing Omarchy
identity. Ordinary use must eventually select the Linux guest without diagnostic
release flags. Keep startup feedback until the guest is actually ready, not just
until QEMU is listening.

Phase 8B local work selects the pinned private Linux guest on ordinary
launches, with an explicit local loopback release fixture. It does not use the
shared Windows guest default. The host waits for a new desktop-ready report
from a guest graphical-session service rather than closing setup at QMP.
The private guest and no-flag Flatpak completed fresh and returning boots in
omabox. Exact final-bundle checks are recorded in the Phase 8B evidence.
Immediate file-drop handoff errors now get a visible dialog; background transfer
failures still need a complete live error path. A public Linux release URL and
independent manifest pin still need an approved distribution plan. The metainfo
now says the VM is stored on disk while the host OS stays in place.

Acceptance: a new user can install, understand storage and permission choices,
launch, change settings, return to the guest and exit using the GUI. Check small
screens, keyboard navigation, accessibility and fractional scaling in isolated
GNOME, KDE, X11 and Omarchy sessions. Audit app metadata for accurate claims.

## Phase 9: everyday feature parity

The first local slice adds saved GUI controls for fullscreen launch and
microphone access; its evidence is in
`docs/evidence/LINUX-SOL-PHASE9-SLICE-2026-09-27.md`. Add GUI controls for
audio routes, broader display preferences, keyboard,
resources/disk capacity, networking/SSH and startup behavior. Implement Linux
camera capture, host battery reporting and the remaining input integrations
where supported. Review host-app launch, USB, gestures and LAN networking
against the Windows and Mac feature inventory instead of silently omitting them.

Finish external file-drop and portal-permission workflows. Brandon chose a
consented Remote Desktop portal session for GNOME Wayland automatic clipboard
sync. Installed GNOME consent, token reuse, two-way text/PNG/file roundtrips and
revocation passed in the Phase 10D private VM. The Phase 10E bundle also passed
GUI file copy and paste in both directions with a file outside its direct
Flatpak filesystem grant. Phase 10F corrected the original GNOME Files drag
finding: native drops reached guest Downloads through an implicit Documents
grant. Fresh small and 64 MiB drops, restart, chooser grant and cancellation
passed on the exact installed Phase 10E bundle. Direct drops into guest apps,
interruption, revoked grants and reconnect remain open. Desktop keyboard-group
changes also remain a real gap. See the
[Phase 10F correction](evidence/LINUX-SOL-PHASE10F-DROP-CORRECTION-2026-09-27.md)
for exact limits.

Acceptance: comparable everyday workflows work through the GUI, saved choices
survive restart, unsupported features have clear behavior, and remaining
platform-specific differences are explicitly accepted by Brandon. No feature
counts as complete merely because its backend or command-line flag exists.

## Phase 10: updates, recovery and installation lifecycle

Not implemented as a complete Linux user experience. Establish the private
Flatpak update and guest-artifact policy and a public Linux distribution plan.
Phase 8B pins a Linux-compatible private guest and exercises ordinary launches
without release arguments against a local loopback fixture. This is still not a
deployable install-and-launch product because the default artifact URL is local.

Provide backup/restore, confirmed reset, relocation, diagnostics and clear
uninstall/data-retention behavior. Reuse safe shared logic where appropriate,
but validate Linux paths, portal grants and Flatpak update behavior separately.
Correct metadata that currently says nothing is installed to disk; the VM does
consume storage even though the host operating system is not replaced.

Phase 10G added an installed GUI path to attach an existing VM after reinstall
or restore-as-copy. A disposable GNOME package removal retained the personal
disk and guest artifacts. The reinstalled app selected the external folder
through the portal, persisted it and booted the retained guest. This is a
partial lifecycle result, not completion of the phase. See the
[Phase 10G report](evidence/LINUX-SOL-PHASE10G-REATTACH-2026-09-27.md).

Phase 10H added a confirmed Forget action for unavailable saved storage. On
the exact installed private Flatpak, a disposable GNOME Wayland VM passed
folder disconnect, reconnect, document portal unexport and regrant, and
retained guest boot. Flatpak `--delete-data` removed app-owned data and the
saved pointer but retained the external VM, shared folder and matched guest
boot files. Reinstall and GUI reattachment booted that VM again. This does not
cover an in-app path-confirming default-VM deletion, desktop Software removal,
physical removable media, updates or rollback. See the
[Phase 10H report](evidence/LINUX-SOL-PHASE10H-STORAGE-2026-09-27.md).

Phase 10I used a new account in a disposable Ubuntu GNOME VM to install the
exact Phase 10H private Flatpak, open it from GNOME search, cancel before
storage selection, and complete default-location personal setup to a visible
Omarchy desktop. A clean guest shutdown and returning launcher start booted
the saved VM without another guest download. The account was fresh, but the OS
and private distribution fixture were preconfigured; graphical install,
trial setup and failure/resume paths remain. See the
[Phase 10I report](evidence/LINUX-SOL-PHASE10I-FIRST-INSTALL-2026-09-27.md).

Phase 10J added an in-app Delete action limited to the default app-owned VM,
with a confirmation naming its data path. The installed GNOME bundle passed
Keep and confirmed Delete on a disposable personal VM; the shared-folder
sentinel and saved preference survived. A subsequent launch remained empty
after Cancel. Private Flatpak rollback and re-upgrade also kept that sentinel,
but ran after deletion and therefore did not test update safety for a retained
VM. See the [Phase 10J report](evidence/LINUX-SOL-PHASE10J-DELETE-2026-09-27.md).

Phase 10K then used a separate child of the retained Phase 10I personal VM.
Phase 10H to 10J app upgrade, rollback and re-upgrade each booted the guest,
and a real guest Documents file kept its SHA256. The saved disk inode and guest
boot pair stayed fixed. One automatic GPU rollback boot had a black window
despite desktop-ready; the installed Software rendering setting recovered it,
and automatic retry later worked. This finding needs repeated exact-candidate
graphics checks. See the
[Phase 10K report](evidence/LINUX-SOL-PHASE10K-RETAINED-UPDATE-2026-09-27.md).

Phase 10N used the installed GNOME GUI to back up, restore as a copy,
reattach and boot a retained personal VM twice. The final restored disk matched
the archive manifest byte for byte, kept the guest boot pair and preserved the
Documents sentinel. A standard Flatpak document grant exposed portal symlink
and unsupported lock behavior during backup; the corrected package passed that
same workflow. Low disk space failed before creating an archive. The full Linux
race suite, including saved-session tests, passed with the packaged QEMU
11.1.1 in the pinned SDK. The [Phase 10N handoff](evidence/LINUX-SOL-HANDOFF-PHASE10N-2026-09-27.md)
separates these installed results from older source checks.

Phase 10O follows an installed restore cancellation that cleaned its staging
copy but left the helper displaying “Cancelling setup...”. The interim helper
accepted the completed home state, but Close remained disabled. Phase 10P
re-enabled that button and passed the installed Cancel, home, Close sequence.
See the [interim finding](evidence/LINUX-SOL-HANDOFF-PHASE10O-2026-09-27.md)
and [final correction](evidence/LINUX-SOL-HANDOFF-PHASE10P-2026-09-27.md).

Acceptance: interrupted downloads/updates, low disk space, corrupt settings,
missing/revoked folders and failed boots have recoverable GUI paths. Updates,
restore and relocation preserve personal files and matched guest boot artifacts.
Keep repositories and distribution endpoints private until publication approval.

## Phase 11: exact-candidate hardening and release decision

Not complete. Freeze a bundle and guest pair, then rerun the full supported
matrix against those exact hashes. Add extended use, repeat launch/reboot,
file-transfer interruptions, audio route recovery, permissions and resource
pressure checks. Repeat visual and input acceptance after fixes. Native Windows
tests remain CI-only; Linux/shared changes still require Windows build and vet.

Physical AMD laptop, real microphone/speakers, suspend/resume, gestures and
multi-monitor checks remain deferred until Brandon asks for that late pass.
Do not treat automated or virtual-desktop checks as physical acceptance.

Publication stays blocked until ordinary use needs no diagnostic flags, the
branding and launcher experience are accepted, core Windows/Mac workflow parity
is demonstrated, serious known issues are resolved, and every remaining gap is
explicitly accepted. Passing those gates still does not authorize publication:
Brandon must separately ask before any push, public PR, issue, site change or
release. Phase numbers are implementation milestones, not a release countdown.

## Current status

Phases 0 through 2 are committed locally. Phases 3 through 7 are implemented
as an uncommitted local candidate, with the acceptance limits below. GTK setup,
resume, portal storage, close confirmation and visibility leases passed live
checks. Clipboard text, images and files passed both directions on Wayland and
X11. Tray settings/shutdown, simulated scaling, private audio and device recovery
passed. The rebuilt guest passed fresh boot and existing-disk migration.

The matrix covers Intel, NVIDIA in an omabox, GNOME, KDE Wayland and Xfce X11.
The final small-window sizing fix was rebuilt and checked on Intel, NVIDIA and
X11; GNOME and KDE checks used the preceding bundle. Private check/build/smoke
automation is ready, and the CI template remains disabled.

Remaining acceptance includes direct drops into guest apps and failure and
recovery paths. GNOME automatic
clipboard passed with consent in the isolated GNOME 46 Wayland VM, including
two-way text, PNG and file transfer, saved permission reuse and a visible
revocation message. Phase 10E then passed an external file grant through the
Documents FileTransfer portal and actual Files copy/paste in both directions
without a broad filesystem grant. Phase 10F found the original dragged file in
guest Downloads and repeated native drops, including 64 MiB, plus the fallback
chooser and cancellation on the exact installed bundle.
Phase 10G then passed ordinary Flatpak removal, external VM retention,
reinstall, portal reattachment and the retained guest boot in the disposable
GNOME VM. Phase 10H passed unavailable storage recovery, portal unexport and
regrant, exact-bundle `--delete-data`, reinstall and retained guest boot.
Phase 10I passed default-location first-run in a new GNOME account. Phase 10J
passed in-app path-confirming deletion for a disposable default VM. Phase 10K
passed private app upgrade and rollback with a retained guest file, with one
recovered intermittent black GPU boot. Phase 10N passed GUI backup and
restore-as-copy on a portal-reattached VM, low-space refusal, and a second
restored guest boot with its personal file. Phase 10P addresses the stuck
post-cancellation window and disabled Close. Graphical installation, guest updates, broader
interruption and recovery remain release gates.
Host keyboard-group switches are not monitored. Physical laptop, real audio,
suspend and real monitor
checks remain deferred. These milestones do not imply release readiness.

See [integration evidence](evidence/LINUX-INTEGRATION-2026-09-26.md) for artifact
identities, verified results and remaining checks.

## Product parity review

See [Linux product parity](LINUX-PARITY.md). The original phases 3 through 7
covered the runtime and initial integrations. They did not include a complete
launcher, feature parity, recovery/update UX or release-level polish. Those are
now explicit phases 8 through 11 and remain outstanding.
