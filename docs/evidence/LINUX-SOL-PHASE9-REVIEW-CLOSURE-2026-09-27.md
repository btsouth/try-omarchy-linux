# Phase 9 Astra review closure

Status: local private checkpoint. The microphone Save retry finding is fixed,
and the packaged late-desktop workflow passed. The three accepted Phase 8B
corrections remain in the source and passed their focused regressions. This is
not Phase 9 completion or release acceptance.

## Exact pair

- Branch `linux-core`, starting HEAD `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`.
  The source delta relative to the Phase 9 slice is
  `/data/try-omarchy-linux-spike/sol-release-continuation/review-closure/source-delta.tar.gz`,
  SHA256 `6d9f3f013538f621e0b5ef18e9c01173007b52f0509787046efe011be144224d`.
  The `source-delta.json` manifest describes 703 current files. The delta was
  captured before this evidence report and later Phase 9B work.
- Flatpak bundle `review-closure.flatpak` in the same directory, SHA256
  `1f22ed32883821b28f4ca3c4d887543add096ed3baaca96fcf956d67b0c3d4f7`.
  Devbox build log: `/workspace/try-omarchy-linux/release-continuation-build.log`.
  The local user installation reported OSTree commit
  `db52d115895d7458cae2aa4d91a785481593cd0e1e6e507c1f4daf81ce385c7f`.
  Its packaged setup helper SHA256 is
  `97fda672c7beffadc66109b1ae80c71809204c3c10bdeba76e0c1059c0a76985`.
- Private guest revision 39 remained unchanged. Its `SHA256SUMS` SHA256 is
  `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`.
  Its manifest SHA256 is
  `a572cdac779f871c38ee4e2048204e131ea34195443815a336256aa7904e04a4`.
  It was served only on task-local loopback port 18090, now stopped.

## Correction and tests

The settings loop now updates its cached microphone preference only after
`desktop-preferences.json` is replaced successfully. A failed write stays
visible on the form and a second Save repeats the write. The regression
fixture forces the first atomic replacement to fail, then retries both
enabling and disabling microphone access. It verifies the disk state and
preservation of camera ID, camera disable and automatic-update preferences.
The focused race run passed, including the Phase 8B clean Cancel, shutdown
handoff and fresh sharing cases. `git diff --check` passed.

The complete unexcluded Linux race invocation passed in a network-isolated
omabox using host QEMU 11.1.1 and a matching task-local `qemu-img` 11.1.1.
Both formerly excluded saved-session tests ran and passed. The sole reported
skip was `TestUSBRealRuntimeControllerAndMissingDevice`: it requires the
Windows runtime's custom `usb-host` `auto-reconnect` property, absent from
Linux QEMU. An explicit `QEMU_SYSTEM` run confirmed that difference, so the
final run used the test's normal runtime detection. Log:
`review-closure/full-linux-race-final.log`. Linux vet, Windows cross-build,
Windows vet with the existing unsafe-pointer suppression, and Windows test
compilation passed. Native Windows tests remain CI-only.

## Live late-desktop result

The exact installed bundle launched a new private personal-account VM in the
`try-omarchy-correction` omabox. The guest reached its real keyboard, username,
password and timezone prompts. Setup was intentionally left at the keyboard
prompt until the five-minute host deadline. The helper showed its timeout
guidance at 02:35:27 while the guest remained running. After disposable account
setup, the guest announced desktop readiness at 02:39:06. The watcher logged
`guest desktop appeared after startup timeout` at 02:39:07. The screenshot
`review-closure/late-desktop-pending.png` shows the rendered Omarchy bar and
wallpaper with the helper gone. QEMU was still running. A subsequent window
close displayed the shutdown confirmation, and choosing Shut down led to
`guest stopped (poweroff)` at 02:41:39. No task QEMU remained.

The VM and screenshots are retained at `/home/bts/try-omarchy-private-late`
and the review-closure evidence directory. The user's real session bus, audio,
clipboard, camera and input devices were not used. The separate private
Omarchy desktop received all GUI input. No commit, push, PR or publication
occurred.
