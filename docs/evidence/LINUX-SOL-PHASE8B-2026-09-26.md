# Linux Phase 8B private review checkpoint, 2026-09-26

This is local work on `linux-core`, not a release. It connects the Phase 8A
home to a private Linux guest and keeps setup visible until the Omarchy desktop
is usable. Phase 9 through 11 parity and release gates remain open. Nothing was
committed, pushed or published, and no project review content was posted.

## Source and recovery

Starting HEAD is `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`, with the
inherited Phase 3 through 7 worktree dirty. The inherited 685-file source
baseline is `/data/try-omarchy-linux-spike/sol-handoff-2026-09-26/source-baseline.tar.gz`
(SHA256 `7577679b68d44c0cdb786e35626652bcfc44a205b3490e02af6bf6bc284a1540`).
Phase 8A's separate delta is `/data/try-omarchy-linux-spike/sol-phase8/source-delta.tar.gz`
(SHA256 `318e16ecf0f36d63a42a0f9972219ab91a20c08a2bf72988364041810a55bc59`).
The Phase 8B delta and file/hash manifest are
`/data/try-omarchy-linux-spike/sol-phase8b/source-delta.tar.gz` and
`/data/try-omarchy-linux-spike/sol-phase8b/source-delta.json`.
They compare to the combined baseline and Phase 8A
state, include untracked files, and do not overwrite the inherited tree. The
final tree has 696 source files; 23 were changed or added since Phase 8A, and
none were removed. `make_source_delta.py` beside the archive reproduces it.

Phase 8B changes: Linux-only guest selection (`app/linux_release_linux.go`),
desktop-ready lifecycle and wait (`app/lifecycle.go`, `app/desktop_ready_linux.go`,
`app/supervise_linux.go`), setup/error handling (`app/main_linux.go`,
`app/linux_errors_linux.go`, `app/runtime_error_linux.go`,
`app/setup_window_linux.go`, `app/ui_linux.go`, `linux-ui/main.go`), the
preboot-settings first-run selection fix (`app/first_run_linux.go`), focused
tests, corrected Linux metainfo, guest patch 0101, and the updated plan,
parity and acceptance documents. The Windows release defaults in
`app/manifest.go` were not changed. The main Windows checkout stayed clean at
`9f9fd43dde4e88aa970a69ca40e05cc94d718738`; the guest-builder checkout
stayed clean at `d5a57b17ca8a58f02257d08ad49cc95960e49ab4`.

## Behavior and parity

| Area | State at this checkpoint | Remaining work |
| --- | --- | --- |
| Phase 8A home and settings | Implemented and visually inspected in Phase 8A; used by the no-flag Phase 8B path. A fresh install can save memory before choosing storage. | Broader Settings controls and desktop visual matrix. |
| Phase 8B first run | Local loopback Linux guest is selected without release arguments. Location, trial account, folder choice, download, boot progress and desktop-ready handoff were exercised in omabox. | Public Linux artifact source and pin; broader failure and accessibility acceptance. |
| Phase 8B returning user | An unchanged image pin boots the saved disk without redownloading. The final pin stages an authenticated image update, then boots an old private disk with its sentinel intact. Setup remains through QMP and userspace readiness until Omarchy's background and bar surfaces stabilize. GUI shutdown confirmation works. | Repeat against public candidate and desktop matrix. |
| Errors | KVM, disk, missing folder, permission, network, verification and early-exit paths have clearer next actions in source/tests. Immediate file-drop handoff failures get a separate visible dialog. | Live transfer-error, portal-revocation, timeout and boot-failure visual acceptance; asynchronous transfer errors still need a visible route. |
| Phase 9 | Inherited clipboard, files, audio, scale and tray foundations remain. | GUI controls, GNOME clipboard, real external drops/grants, camera, battery, keyboard-group changes, host-app/USB/LAN scope. |
| Phase 10 and 11 | A private guest is pinned; exact local pair can boot. | Public artifact/update policy, backup/reset/move/diagnostics and full exact-candidate matrix. |

Evidence labels for the final pair: **implemented, automated-tested and
visually inspected** for the no-flag first run, old-disk guest update,
desktop handoff, shutdown and local 404. **Automated-tested only** for the
other setup advice, timeout transitions and immediate file-drop dialog.
**Unsupported** today: GNOME automatic clipboard and Linux camera/battery
feeds. **Decision-needed**: host-app launch, USB and LAN scope. **Physical-
unverified**: the laptop, real audio and microphone, camera, suspend, gestures
and monitors.

The default URL is loopback, so this bundle cannot be used as a public install
candidate. Physical checks remain separate before a release decision.

## Exact artifacts and matrix boundary

| Artifact | Identity |
| --- | --- |
| Final review Flatpak | `/data/try-omarchy-linux-spike/sol-phase8b/com.tryomarchy.TryOmarchy.flatpak`, SHA256 `713aad5c7f2bdd7da6875bdc9eb720c44939007a454c9acb56386c899e4e8cc8` |
| Local installed OSTree commit | `dbb72cc67fbd582af530d6516a70823da4049bb36e3e532b2f44fb15076abc7e` |
| Packaged launcher / GTK helper | SHA256 `0a77f077add082ff523e546ae6f509a73bc408ae9f1638ff3d1f12fb0bec6b4c` / `2a1764463745090e3f8a14c3f3ec9e78db756cf6c85e438af2c08a27cd935f5f` |
| Private guest | `/data/try-omarchy-linux-spike/sol-phase8b/guest-r3/`; `guest-manifest.json` SHA256 `a572cdac779f871c38ee4e2048204e131ea34195443815a336256aa7904e04a4`; trusted `SHA256SUMS` SHA256 `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`; compressed rootfs SHA256 `afce576526c826c7fb19eec08988469efe2ab41262c3c7ab253315ad76b6e108` |
| Guest source/build | Builder base `d5a57b17ca8a58f02257d08ad49cc95960e49ab4` plus local patch 0101 (SHA256 `9f1a059873fc48cd36eb9f56d009737af21d92d97ec760266764143e48dd9c8d`); compatibility revision 39, kernel 7.2.7-arch1-1; builder image `sha256:6d96efcad207b733f365973d72e5f9df39d549fca2a5037d43e9fb8749f91228`; build-spec SHA256 `57971e14d323392ecc592129be488250e9f9f9b6c0394e614246f3aa40604fdb` |
| Runtime/hosts | GNOME 50 Flatpak runtime, QEMU 11.1.1, virglrenderer 1.3.0; local Flatpak 1.18.3 and Omarchy kernel 7.2.5-4; devbox Flatpak 1.14.6 and Ubuntu kernel 6.8.0-139 |

| Row | Pair and actual result |
| --- | --- |
| Initial Phase 8B package | Bundle SHA256 `4feead007ca6d69ac1a70f1ab633efcf6481d8bd561f1aaf5538a4cfa155103d` plus the private guest: fresh and returning no-flag boots reached the visible desktop; GUI shutdown passed. This bundle predates later error text and UI fixes. |
| Intermediate exact rebuild | Bundle SHA256 `6d62d8c1a1bb7d045a871dfbd77d7391635e4c637fcd26b21f98eeab126136a8` plus the same guest: returning boot and GUI shutdown passed. A fresh path with Settings saved first failed because the location chooser rejected its own `settings.json`; this was fixed before the final review bundle. |
| Pre-layer-check package | Bundle SHA256 `3c91a4d9183adb07a594a59ad58a215e2df94294e6e730fd4bd283af6d93bbf7` plus the revision 38 guest: fresh and returning no-flag boots and a personal-data sentinel passed. It exposed an early desktop-ready signal: the window first showed black and gained the bar/wallpaper seconds later. The final guest waits for those surfaces. |
| Final review pair | Bundle SHA256 `713aad5c7f2bdd7da6875bdc9eb720c44939007a454c9acb56386c899e4e8cc8` plus the revision 39 guest, with no release arguments: the old private disk updated its guest image, booted, reported revision 39 and kept the sentinel; a fresh disk reached a rendered bar/wallpaper at handoff and shut down through the GUI. A local 404 source displayed an actionable error. |

Guest files were verified against `SHA256SUMS` on devbox, including the
uncompressed 7 GiB rootfs. The local compressed release copy matches its
manifest; the local copy omits the uncompressed rootfs. The built rootfs
contains the systemd user service and compatibility revision 39. Patch 0101
reapplies cleanly with `git am` in an isolated clone.
The first direct guest build hit Docker bridge DNS failures; a retry with the
retained verified builder image and host networking succeeded. The existing
guest-builder checkout stayed clean. Final guest build, artifact check and
Flatpak build logs were copied from devbox to
`/data/try-omarchy-linux-spike/sol-phase8b/logs/`; their devbox originals are
`/workspace/try-omarchy-linux/phase8b-guest-build-r3.log`,
`phase8b-guest-r3-checks.log` and `phase8b-flatpak-layer-ready.log`.

## Tests and visual evidence

Local source checks after the final code change: `go test -race -count=1 -skip
'TestSavedSessionRAMSurvivesNewQEMUProcess|TestSavedSessionPublishesMatchedDiskAndRAM'
./...`, `go vet ./...`, Windows `GOOS=windows GOARCH=amd64 go vet
-unsafeptr=false ./...`, Windows cross-build and test compilation passed.
The two skipped saved-session tests use QMP features unavailable in devbox's
QEMU 8.2.2; the unmodified suite was tried earlier and failed there. Native
Windows tests were not run. Exact final source logs and compiled Windows binaries
are under `/data/try-omarchy-linux-spike/sol-phase8b/checks-final/`.
`git diff --check` passed. Guest patch tests:
`python3 -m unittest discover -s guest/tests -p test_desktop_ready.py -v`
passed four cases. The final Flatpak build log is in the copied logs above.

All GUI screenshots below are from the isolated `try-omarchy-sol-8b` omabox,
not the real desktop. The no-flag launcher uses a private `XDG_DATA_HOME` and
local server on `127.0.0.1:18090`; the real Flatpak default data folder stayed
absent. Screenshots and launcher logs are under
`/data/try-omarchy-linux-spike/sol-phase8b/`.

- Exact final first-run: `layer-fresh-home.png`, `layer-fresh-settings.png`,
  `layer-fresh-location.png`, `layer-fresh-account.png`,
  `layer-fresh-share.png`, `layer-fresh-download.png`,
  `layer-fresh-at-ready.png`, `layer-fresh-desktop.png`,
  `layer-fresh-shutdown.png`; launcher log `layer-fresh.log`.
- Exact final existing disk: `layer-existing-home.png`,
  `layer-existing-update.png`, `layer-existing-desktop.png`,
  `layer-existing-sentinel.png`, `layer-existing-shutdown.png`;
  launcher log `layer-existing.log`.
- Exact final error: `error-404.png` and `error-404.log` show an unavailable
  local image source and a next action, with no VM started.
- The immediately preceding `3c91a4d9` bundle has the **same GTK helper SHA256**
  as the final bundle. Its `visual-final-location.png`,
  `visual-final-share-small.png`, `visual-final-share-small-scroll.png`,
  `final-home-dark.png` and `final-settings-dark.png` show the 960x720 light
  path, 800x480 scrolling with reachable footer, and dark home/Settings.
  Keyboard Return on the exact final home opened the location choice.
- Initial bundle's full first-run, download, desktop and shutdown screenshots:
  `home.png`, `location-ready.png`, `account-ready.png`, `share.png`,
  `download-start.png`, `booting.png`, `desktop-first.png`,
  `shutdown-confirm.png`, `home-return.png`, `desktop-return.png`,
  `shutdown-return.png`. The intermediate rebuild's returning screenshots are
  `exact-home.png`, `exact-return-booting.png`, `exact-return-desktop.png`,
  `exact-shutdown-confirm.png`; its failed fresh path is
  `exact-first-account.png`.

The initial bundle's first boot reported userspace-ready before desktop-ready;
setup stayed until Omarchy's bar and wallpaper were visible. Its returning disk
made no new guest HTTP requests. The intermediate rebuild likewise reached the
visible desktop and shut down after confirmation. The later revision 38 fresh
boot exposed a black frame immediately after its premature `desktop-ready`
report. Patch 0101 was strengthened to require stable `omarchy-background` and
`omarchy-bar` surfaces and compatibility was raised to 39. In the final fresh
boot, `layer-fresh-at-ready.png` shows both surfaces already rendered while the
helper transitions away; `layer-fresh-desktop.png` shows the unobstructed
desktop. The guest then shut down after GUI confirmation.

An old revision 38 private disk received the revision 39 image, booted and
reported `39:7.2.7-arch1-1` inside the guest. Its installed readiness script
contained the background-layer check. The file
`~/Documents/phase8b-sentinel.txt` still hashed to
`a0140892991281ce57c0af6d59fb7c9c9645f103af48dcaedbf57d6bc017d9fa`,
matching its pre-update value. Portal permission persistence and live external
file drops were **not rerun** with the final pair.
Earlier cancellation, helper-failure, portal-grant and sentinel evidence is in
`LINUX-INTEGRATION-2026-09-26.md`; it belongs to earlier bundles, not this pair.

## Serious issues and next bounded task

| Issue | Impact and next action |
| --- | --- |
| Private guest distribution | The loopback URL and checksum pin work locally but have no public source. Plan a Linux release location, independent trust pin and update/rollback policy before an install candidate. |
| Remaining Phase 8B acceptance | Complete exact-candidate GNOME/KDE/X11 visual, screen-reader/focus, revoked grant, boot-error and timeout checks. At 800x480 the footer remains reachable, but the complete description needs scrolling. The local HTTP 404 is the only live failure view on this bundle. |
| File handoff and GNOME clipboard | Immediate file-drop errors have a visible dialog in source and fixture tests; the live drop path and background transfer failures remain open. Real external portal grants are untested on this pair. GNOME automatic clipboard is unsupported. Implement and test the supported path, then document any accepted restriction. |
| Everyday parity | Add Phase 9 GUI audio/mic, display/input, networking and storage controls; camera/battery; review host-app, USB, gestures and LAN scope against Windows/Mac. |
| Recovery and release hardening | Add Linux backup/reset/move/diagnostics, Flatpak update/rollback policy, exact desktop matrix and physical acceptance. The authenticated private guest image update passed, but it is only one part of update lifecycle acceptance. No near-parity or release claim yet. |

The next bounded implementation task is Phase 9's everyday GUI controls and
external file/clipboard permission path, while Phase 8B matrix and error-state
checks remain tracked. Physical acceptance and release approval remain separate.

## Cleanup

The task omabox `try-omarchy-sol-8b`, its launcher/helper/VM processes, and
the loopback guest and 404 servers on port 18090 were stopped. No task Flatpak
or QEMU process remained; the real default Flatpak data folder remained absent.
The local user Flatpak installation was updated to OSTree commit
`dbb72cc67fbd582af530d6516a70823da4049bb36e3e532b2f44fb15076abc7e`.
No host tool/runtime package was installed. Devbox retained the pinned build
image, Flatpak cache, guest build volumes and logs. Local source, bundles,
compressed guest releases, screenshots, logs and private test disks remain
under `/data/try-omarchy-linux-spike`; `/data` had about 60 GiB free after
retention. No worktree commit or external publication occurred.
