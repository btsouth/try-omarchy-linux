# Phase 9 local slice: display and microphone choices

Status: implementation checkpoint for Astra review, not Phase 9 acceptance or
public release readiness. The Phase 8B correction is frozen separately in
`LINUX-SOL-PHASE8B-CORRECTION-2026-09-27.md`. No commit, push, tag, deployment
or publication occurred.

## Exact candidate

- Checkout: `/home/bts/Projects/try-omarchy-linux-core`, branch `linux-core`,
  starting HEAD `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`. The Phase 9
  source delta relative to the Phase 8B correction is retained at
  `/data/try-omarchy-linux-spike/sol-phase9-slice/source-delta.tar.gz` with
  `source-delta.json` beside it.
- Flatpak: `/data/try-omarchy-linux-spike/sol-phase9-slice/phase9-slice-final.flatpak`,
  SHA256 `158d8f3d08fdedb13839cde0158da0ddff89f70b24b9b5ef48646570bc19b969`.
  Built on `devbox` from the final Phase 9 implementation source. Its copied
  hash matched the devbox bundle. Build log:
  `/workspace/try-omarchy-linux/phase9-slice-build-final.log`. User Flatpak
  installation reported commit
  `08dfb64d0c94177707f0ccb1f7e5caaf9e5b8d53eb6529fcfcab82361fec4c0b`.
- Guest revision 39 and its private loopback source are unchanged from the
  correction checkpoint. This does not establish a public download path.

## Implementation and evidence

The Linux Settings page now exposes `Open Omarchy fullscreen` and `Allow
microphone access`. Both choices apply on the next VM launch, as the page
states. Fullscreen saves to `settings.json`; microphone access saves to
`desktop-preferences.json`, preserving camera and update preferences. The
first-run custom storage handoff now carries both files, so a preboot
microphone choice is not lost when the user chooses another location.

`TestLinuxDisplayAndMicrophoneSettingsSurviveRestart` exercises Save, reload,
preservation of unrelated desktop preferences, and custom storage handoff.
The existing Linux audio tests cover `in.voices=0` when microphone access is
disabled. The focused Phase 8B correction tests remain green.

In the task omabox at 1280 by 800, `settings-initial.png` and
`settings-changed.png` show the new checkboxes. `settings-scrolled2.png` shows
the shared-folder field and switch still reachable below them. The saved
files contain `fullscreen: true` and `microphoneDisabled: true`, and
`settings-restarted.png` shows those same choices after closing and reopening
the app. The QEMU command in the private disk's `vm/shell.log` contains
`-full-screen` and, on its initial PipeWire attempt,
`-audiodev pipewire,id=snd,in.voices=0`. The contained session has no host
PipeWire service, so QEMU retried with silent audio. `fullscreen-desktop.png`
shows the guest bar and wallpaper filling the omabox display. The fullscreen
window's close confirmation led to `guest stopped (poweroff)` at `01:44:58`.
Actual microphone capture and route selection were not tested here.

The personal setup run in the correction checkpoint exposed a stale timeout
message after the desktop appeared late. The watcher now closes that message
when desktop readiness arrives after timeout. The new
`TestLinuxLateDesktopClosesTimeoutMessage` verifies that the helper closes
without treating the late desktop as cancellation. The earlier live timeout
reproduction belongs to the correction report; this follow-up behavior has
deterministic coverage but has not had a second five-minute live setup run.

## Checks

- Full `go test -race -count=1` for the app packages passed inside a throwaway
  omabox with the two previously documented QEMU 8.2 saved-session tests
  excluded: app `34.379s`, sign-update `1.023s`. It is not a full unexcluded
  suite pass.
- Focused race tests for new settings, custom storage, late desktop timeout
  and pending shutdown handoff passed in omabox.
- Linux `go vet ./...`, Windows cross build, Windows
  `go vet -unsafeptr=false ./...`, and `git diff --check` passed. Native Windows
  tests were not run. The correction checkpoint separately compiled Windows
  tests.
- The private VM and app exited. The user's real desktop was not used. The
  task omabox, private disks, logs and screenshots remain local for review.

## Remaining Phase 9 and release gates

Audio route selection and live switching, broader display/monitor and
keyboard controls, disk capacity, networking/SSH/startup GUI, camera, battery,
input integrations, external portal/drop work, and the GNOME clipboard gap
remain open. The personal setup timeout auto-dismissal needs a final live
reproduction. Phase 10 update commit/rollback and recovery, public guest
distribution, desktop matrix, accessibility, physical acceptance and an exact
release candidate remain open. No Linux public release is recommended.
