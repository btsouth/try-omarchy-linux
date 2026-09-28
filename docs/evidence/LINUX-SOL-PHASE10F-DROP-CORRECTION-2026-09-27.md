# Phase 10F private GNOME file-drop correction, 2026-09-27

## Verdict and correction

The installed Phase 10E Flatpak **does receive native GNOME Files drops**. The
Phase 10E report incorrectly inferred failure from the absence of a success
line in `shell.log` and a chooser. A dropped `host-drop-世界.txt` was later
found in guest Downloads with the timestamp of that original drag. GNOME Files
had supplied a Documents FileTransfer grant, so the app could transfer the file
without opening its fallback chooser. The app logs drop errors and cancellation
but currently does not log successful drops.

This run repeated the result with fresh files in a normal, uninstrumented
installed app. A separate URI-only GTK drag fixture, which does not offer the
portal FileTransfer key, exercised the explicit chooser and cancellation paths.
No product source or bundle changed in Phase 10F. This corrects evidence and
narrows the remaining release gap; it does not complete Phase 9 or release
acceptance.

## Exact candidate and isolation

- Checkout `/home/bts/Projects/try-omarchy-linux-core`, branch `linux-core`,
  starting HEAD `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`. Existing
  uncommitted product work was preserved. Phase 10F adds only this report,
  fixture and desktop evidence, and corrections to the existing ledgers.
- Installed bundle: [`phase10e/phase10e-final.flatpak`](phase10e/phase10e-final.flatpak),
  SHA256 `0c8386ca78a5b1369f9269bc992a5d09c6bda0880c594adaee6fc895d785a05a`.
  GNOME Flatpak OSTree commit remained
  `ebf133d71595c92892b03d6c24a53443ff4a07eb1a5936f0b600e83c378bddc8`.
  The Phase 10E source delta remained
  `d531c8e51e2a621d2f28bceb5c865454c3047e7147145f668cd66c6ff1412d15`.
- Guest was the existing private compatibility revision 39 and personal disk
  from Phase 10E. Its inherited artifact hashes are in the
  [Phase 10E report](LINUX-SOL-PHASE10E-FILE-PORTAL-2026-09-27.md); they were
  not recomputed here. The guest shut down cleanly after the final tests.
- GNOME 46 Wayland ran in a disposable devbox VM with an overlay on the
  retained matrix base. A pre-debug overlay copy is at
  `/workspace/try-omarchy-linux/phase10e-gnome-live/desktop-overlay-before-drop-debug.qcow2`,
  SHA256 `06e04c47286a479c291a395781898566c8829c947927c3adffcd0381ff31b87b`.
  The final overlay is `desktop-overlay.qcow2` in that directory, SHA256
  `2357984e3edf2984c6ac28d1bd24dea962865ec9d44bd76540c7ff079221108f`.
  The VM and guest are stopped; the QEMU PID file was removed. Only this
  task's `phase10f-drop` omabox and VNC tunnel were stopped. No real desktop
  session, bus, clipboard, audio, camera or input device was used.

## Installed GUI observations

| Path | Observation |
| --- | --- |
| Native GNOME Files drag | Fresh outside files `grant-phase10f-世界.txt` and `cancel-phase10f-世界.txt` reached guest Downloads without a chooser. Each guest SHA256 matched its source. The second name describes an attempted cancellation fixture, but GNOME automatically granted and transferred it, so it is **not** cancellation evidence. |
| Large native drag | Outside `large-phase10f-64MiB.bin` reached guest Downloads at exactly 67,108,864 bytes; source and guest SHA256 were `26579468d8ed129d38bb7b5e76e111c25bc695692438cd25dfc482e8c7d4aaa2`. The Flatpak had no direct access to the outside source path. See [host source](phase10f/large-source.png) and [guest Files after restart](phase10f/restart-guest-downloads.png). |
| Restart | After a clean guest and app restart with no diagnostic preload, another native GNOME Files drop, `restart-phase10f-世界.txt`, reached guest Downloads with SHA256 `bf56035ee9a2c8591b19223631da85c02e995b7eb44c472d5a26651b860e4408`. See [guest Files](phase10f/restart-guest-downloads.png). |
| Explicit chooser grant | The private [URI-only drag fixture](phase10f/uri_only_drag.py) offered `text/uri-list` without a FileTransfer key. Dropping an outside file opened the app's [Share dropped files](phase10f/uri3-prompt.png) prompt. Choosing the file in the [portal chooser](phase10f/chooser-selected-proper.png) and pressing Share transferred `chooser2-phase10f-世界.txt` to guest Downloads; source and guest SHA256 were `dfc36ef4e571baf38cfeca247960c0af646a294692ac0e1944c3bf7e6cee2b9a`. The direct Flatpak sandbox path test exited successfully for `test ! -e` on that source. |
| Cancel | A fresh URI-only `cancel2-phase10f.txt` drop was cancelled at the app prompt. The prompt [dismissed](phase10f/cancel-dismissed.png), the guest file remained absent, and `shell.log` recorded `file drop: cancelled`. A separate fresh `portal-cancel-phase10f.txt` drop opened the portal chooser; its Cancel returned to the app with [File access was not granted](phase10f/portal-cancel-return.png). Cancelling the app prompt left the guest file absent and logged cancellation. |

Source/guest hashes for the two small automatic drops were respectively
`19c3b37462484a6606b648fa3c4477069bae44c8bee8d5eda61a89fb5888ca8c`
and `708f19489efc2d4fc24b3044408c7befb7150f9fbe1252a18a4fccd4baa96173`.
The final [installed shell log](phase10f/installed-shell.log) shows cancellation
and clean poweroff. It is not a successful-drop audit log. The first explicit
chooser attempt submitted with no selected file and showed `the file chooser
did not grant any files`; manually selecting the file then passed. Diagnostic
SDL event tracing used a test-only preload in an earlier run, but its trace
file was overwritten on restart. The uninstrumented file and hash checks above
establish the result without relying on that trace.

## Remaining limits and next actions

- This proves drops to the VM's guest Downloads target. Direct drops into a
  guest application, as offered by newer Windows work, were not tested.
- Revoked Documents grants, transfer interruption, cancellation during a large
  active transfer, and reconnect after portal failure still need installed
  acceptance. The larger desktop matrix must be repeated against one frozen
  candidate. No physical checks were performed.
- Continue Phase 9 everyday control and host-integration gaps, then Phase 10
  install, uninstall, update and recovery lifecycle. Preserve this exact bundle
  and source delta for reconstruction; do not substitute a rebuild for it.

Artifact hashes for this report's fixtures, logs and screenshots are in
[`phase10f/artifact-sha256.txt`](phase10f/artifact-sha256.txt).
