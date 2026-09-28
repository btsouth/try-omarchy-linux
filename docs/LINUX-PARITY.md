# Linux product parity, 2026-09-27

Status: private engineering candidate, not ready for publication. Phase 8A
added the pre-boot home and preference controls. Phase 8B pairs it with a
private Linux guest and waits for the usable desktop. The Phase 8B correction
and first Phase 9 slice have separate local evidence reports. Physical tests
remain deferred.

The public release references currently resolve to Windows
[v0.6.0](https://github.com/omacom/try-omarchy-windows/releases/tag/v0.6.0) and Mac
[v0.4.1](https://github.com/omacom/try-omarchy/releases/tag/v0.4.1). The
[Mac main README](https://github.com/omacom/try-omarchy/blob/main/README.md) also
provides the current implementation target; main can contain work newer than
the released app. This is not a claim that every main-branch feature has shipped
or passed hardware acceptance. The older MAC-PARITY.md remains historical
Windows/Mac evidence and should not determine current release status by itself.

| Area | Linux today | Work before near parity |
| --- | --- | --- |
| Identity | Same Omarchy SVG icon, Try Omarchy title, desktop entry and tray identity; Phase 8A adds version/help in the idle home | Accurate metadata, full visual acceptance and broader product polish |
| First run | GTK location, trial/personal account, shared folder, download progress and cancellation; private Phase 8B guest reached the desktop without release flags | Broader error/recovery paths, portal grants, accessibility and desktop matrix |
| Returning user | Idle home offers Launch, Settings, saved disk status and Use existing data folder; Phase 10G installed GNOME bundle reattached an external retained VM through the folder portal and booted it after reinstall | Removable-drive failure/regrant and broader startup/display preferences and desktop matrix |
| Settings | Automatic or bounded manual RAM/CPU controls, rendering, sharing, fullscreen launch and microphone access, with saved choices | GUI audio routes, broader display, input, networking and storage controls; broader accessibility review |
| Core VM | KVM, GPU/OpenGL, conditional Venus, CPU fallback, reboot/shutdown and RAM reclamation | Broader workload, graphics and long-running acceptance |
| Clipboard/files | Text, PNG and file roundtrips on tested Wayland/X11; selected shared folder; installed GNOME Wayland portal passed consent, token reuse, two-way text, PNG and file transfers, and visible session revocation; Phase 10E installed GUI copy/paste passed for an external file without a broad filesystem grant; Phase 10F verified native GNOME Files drops to guest Downloads, 64 MiB transfer, explicit chooser grant and cancellation | Direct drops into guest apps, transfer interruption, revoked grants and broader desktop reconnection |
| Audio | PipeWire playback/recording and dummy-device recovery; microphone GUI choice and route flags | User-facing device choices and live switching parity; physical acceptance later |
| Camera and battery | Camera capture unavailable; host battery sysfs bridge has fixture coverage | Packaged battery and real supported-device acceptance; camera capture integration |
| Input/display | Focused keyboard capture, simulated scale following, environment/flag keyboard layout, fullscreen GUI choice | Layout switches, gestures, broader monitor UX and physical coverage |
| Networking | NAT and loopback port/SSH flags | GUI controls and a deliberate LAN/bridge scope decision |
| Recovery/updates | Installed GNOME GUI backup and restore-as-copy of a portal-reattached personal VM, matching archive and restored disk SHA256, preserved guest Documents file and boot pair; low-space refusal; Phase 10P restored GUI usability and Close after cancellation; prior Phase 10G uninstall/reattach, 10J confirmed default deletion and 10K app upgrade/rollback passed | GUI reset/move, guest update/rollback, interrupted replacement/deletion, other storage locations and graphics stability |
| Other host integration | No Linux approved-host-app launcher or USB management UI | Inventory Windows/Mac behavior and explicitly decide Linux scope |
| Distribution | Locally built/installed Flatpak pins a Linux guest served on local loopback | Public artifact location and pin policy, private update rehearsal |

Phase 3 through 7 screenshots: `/data/try-omarchy-linux-spike/phase3-flow/devbox-account-final.png`
and `/data/try-omarchy-linux-spike/phase5/kde-settings.png`. They show the prior
GTK form with technical text fields. Phase 8A replaces those source controls;
its exact visual evidence and remaining gaps belong in the Phase 8 report.

Phase 8B selects a pinned private Linux guest from local loopback on ordinary
launch, rather than the shared Windows release defaults. This is a test fixture,
not a public distribution channel. Successful earlier Linux acceptance used the
private guest with patches 0096 through 0100 and explicit release/checksum
arguments. The new desktop-ready guest patch is 0101. The exact paired runtime
result is in the Phase 8B evidence. A public Linux artifact plan remains a
release gate.

Near parity means comparable everyday outcomes and a usable GUI, not identical
platform APIs. Mac Touch ID and Windows Hello cannot be copied verbatim to Linux;
any equivalent or deferral needs an explicit scope decision. Likewise, platform
restrictions do not automatically excuse missing core workflows. Keep the GNOME
clipboard workflow open until consent and two-way roundtrips pass on the installed package.

The plan now has phases 8 through 11 for product polish, everyday parity,
installation/recovery/update work and exact-candidate hardening. Nothing should
be published just because phases 3 through 7 have implementation evidence.

## Dated comparison for the private candidate

Checked 2026-09-27 against the published [Windows v0.6.0
release](https://github.com/omacom/try-omarchy-windows/releases/tag/v0.6.0),
its [tagged README](https://raw.githubusercontent.com/omacom/try-omarchy-windows/v0.6.0/README.md),
and the published [Mac v0.4.1
release](https://github.com/omacom/try-omarchy/releases/tag/v0.4.1) and
[tagged README](https://raw.githubusercontent.com/omacom/try-omarchy/v0.4.1/README.md).
The tagged README is a description of shipped source, not a fresh physical
acceptance result. The Windows v0.6.0 notes add direct drops into guest apps,
Windows Hello for 1Password, live local port-forward changes and update fixes;
v0.5.0 added Windows Hello for sudo. The Mac v0.4.1 notes fix a macOS 15 launch
crash and theme/DNS password dialog. Newer main-branch work is a separate target
and is not counted as a released feature.

| End-user outcome | Windows v0.6.0 | Mac v0.4.1 | Linux private work on 2026-09-27 |
| --- | --- | --- | --- |
| Normal install and first desktop | Signed launcher, verified download and guided first run | Signed DMG and app, guided guest start | Phase 10I installed the exact private Flatpak in a clean Ubuntu GNOME account, opened it from the launcher, completed default-location personal setup to a visible guest desktop and returned after clean shutdown without another download; Phase 10G/10H covered separate uninstall, reattach and storage recovery paths; Phase 10J passed default-VM deletion; clean OS and graphical distribution route remain open |
| Audio and microphone | Live route switching; microphone switch at next start | Live route switching in guest | GUI route choices and microphone switch in Phase 9B bundle; no live switching; private audio recovery and physical devices still need acceptance |
| Camera and battery | Camera controls and host battery feed | Portal-style host consent and on-demand virtual camera | Camera guest bridge reports unavailable; Linux battery sysfs source added after Phase 9C snapshot, fixture tested, packaged and VM acceptance pending |
| Clipboard and files | Two-way text/image/file clipboard, direct drop into guest apps and sharing | Two-way text/image clipboard and chosen shared folder | Installed GNOME 46 Wayland consented portal passed two-way text, PNG and files; Phase 10E Files copy/paste passed in both directions with an external file and no broad grant. Phase 10F native Files drops reached guest Downloads, including 64 MiB, with chooser and cancel fallback checked. Direct drop into guest apps remains untested |
| Display and input | Fullscreen monitor choice, capture release, Precision Touchpad pinch | Native resize and HiDPI updates | Fullscreen and scale/layout preferences; packaged personal desktop booted at 150% and German layout; multi-monitor and gestures open |
| Networking and host integration | NAT, SSH/port forwards including live local changes, approved Windows app launch, experimental portable USB storage and Windows Hello guest sudo/1Password | NAT/bridge and SSH; Touch ID guest sudo | NAT and SSH GUI source in Phase 9C build; LAN, live forwarding, approved host-app launch, USB device access and host authentication outcome not implemented |
| Data and recovery | GUI backup, reset, move, diagnostics, signed update recovery | VM location and reset, Mac update path | Installed Phase 10N GUI backed up a portal-reattached VM, restored an independent copy, reattached and booted it with the same personal file and guest boot pair. Low space refused before output. Phase 10O interim showed a corrupt-ZIP error; Phase 10P Cancel returned home, removed staging and allowed Close. Earlier Phase 10G/10H reattachment and storage recovery, 10J confirmed deletion and 10K app upgrade/rollback passed. One intermittent black GPU boot remains open. GUI reset/relocation, guest updates and broader recovery remain open |

The [Phase 10H evidence](evidence/LINUX-SOL-PHASE10H-STORAGE-2026-09-27.md)
distinguishes the exact installed Flatpak's missing-folder and portal-regrant
behavior from physical removable-drive acceptance. Removing a GNOME document
permission entry alone did not immediately revoke an already exported path;
document unexport did. The installed home then reported the missing path and
reattached the same disk under a new portal ID. No equivalent Windows or Mac
release feature is claimed from this Linux-only recovery check.

The [Phase 10N backup and restore evidence](evidence/LINUX-SOL-HANDOFF-PHASE10N-2026-09-27.md)
includes a real installed backup from a portal-reattached VM, a second
independent restore, exact disk and boot-file hashes, visible guest boot and
matching personal file. The [Phase 10P correction](evidence/LINUX-SOL-HANDOFF-PHASE10P-2026-09-27.md)
adds installed cancellation and Close recovery. These are narrower than full
Windows recovery parity: reset/move, guest update/rollback and interrupted
replacement are still unverified. The home view currently exposes an opaque
document-portal storage path instead of the selected host folder name.

Brandon chose consented automatic GNOME Wayland clipboard synchronization on
2026-09-27. The source now creates a [Remote Desktop
session](https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.RemoteDesktop.html)
with pointer permission because GNOME disables Share for a clipboard-only
request, requests the [Clipboard
portal](https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.Clipboard.html)
before Start, requires desktop consent and stores a single-use restore token
when offered. It does not send remote input events. This broader GNOME consent
is described in Settings and About. The installed GNOME 46.2 portal prompt,
token reuse after reinstall/restart, two-way text/PNG/file transfer, and visible
session revocation passed in the disposable Wayland VM. The private D-Bus
regression also covers GNOME's struct-wrapped MIME list. The file roundtrip
used a test-only Flatpak filesystem grant to the fixture folder. Phase 10E
subsequently passed an actual external-file grant and GUI copy/paste in both
directions. Phase 10F corrected the native drop result and passed fresh drops,
a 64 MiB file, chooser grant and cancellation on the installed bundle. See the
[Phase 10D evidence](evidence/LINUX-SOL-GNOME-PORTAL-2026-09-27.md) and
[Phase 10E evidence](evidence/LINUX-SOL-PHASE10E-FILE-PORTAL-2026-09-27.md)
and [Phase 10F correction](evidence/LINUX-SOL-PHASE10F-DROP-CORRECTION-2026-09-27.md).
