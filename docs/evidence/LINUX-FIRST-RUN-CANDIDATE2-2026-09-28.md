# Candidate 2 first-run continuation

Candidate 2 passed the resumed Ubuntu GNOME trial setup, keyboard controls,
shutdown and returning-launch checks on September 28, 2026. No application
code changed during this continuation. This is private VM evidence, not a
publication or physical-hardware acceptance record.

## Candidate and test environment

- Checkout: `linux-first-class`, `9a77dd6ba270c5ad1a4c8b05726265255aa9fc30`.
  This includes the fix that sends the home screen its action buttons.
- Installed OSTree: `bb826369ccb014ac490dc1ad49f3eca4b80976b98388218a0039c93d586fc178`.
  It matches `/workspace/try-omarchy-linux/fc-site2/commit.txt` on devbox.
- Candidate files: `/workspace/try-omarchy-linux/fc-release2` on devbox.
  All three files passed their [SHA256 checks](first-run-candidate2/artifact-verification.txt);
  [artifact hashes](first-run-candidate2/artifacts.txt) are retained.
- The local `app/` and `linux-ui/` files byte-match the devbox build source
  in `/workspace/try-omarchy-linux/fc-cand2-src`, checked with checksum-based
  rsync dry runs.
- Disposable Ubuntu 24.04 GNOME VM: devbox
  `/workspace/try-omarchy-linux/fc-clean/desktop-overlay.qcow2`, user `ana`.
  Its launcher forwards SSH on port 22260 and VNC on 5921. The existing
  `cleanview` omabox holds the viewer. The physical desktop was not used.
- The installation page is an HTTPS stand-in inside the Ubuntu VM. A browser
  address showing `btsouth.github.io` here is not evidence of a public deployment.

## Resumed journey

The prior session reported installing candidate 2 through Software in about
37 seconds, including the runtime. That timing was not remeasured. This run
resumed at the [clipboard explanation](first-run-candidate2/resume.png), before
any guest download had started, and verified the installed commit independently.

| Check | Result |
| --- | --- |
| GNOME consent | Continue opened [Remote Desktop](first-run-candidate2/consent2.png). Clipboard access was already on; enabling remote interaction enabled [Share](first-run-candidate2/consent-enabled.png). |
| Download and preparation | The UI showed [download progress](first-run-candidate2/progress.png) and [verification](first-run-candidate2/current.png). The download ran from 20:56:38 to 20:57:16 UTC, then unpacking, verification and disk preparation completed. |
| First boot | The [trial desktop](first-run-candidate2/ready.png) rendered. Desktop readiness was recorded at 20:58:31, 1 minute 53 seconds after accepting clipboard sharing. GNOME's shortcut-inhibition prompt was allowed once. |
| First-session tip | The [notification](first-run-candidate2/release-keys.png) appeared and its marker was written at 20:58:31 UTC. |
| Keyboard release | Ctrl+Alt+G removed the grab hint from the title. Super then opened the [Ubuntu overview](first-run-candidate2/host-shortcut.png). Clicking the guest restored the grab, and Super+Space opened [Omarchy's menu](first-run-candidate2/guest-menu.png). |
| Fullscreen | Ctrl+Alt+F entered [fullscreen](first-run-candidate2/fullscreen.png) and returned to a window. |
| Close and keep | Closing opened the [shutdown confirmation](first-run-candidate2/shutdown-prompt2.png). Keep running dismissed it and preserved the running desktop. |
| Clean shutdown | Shut down requested poweroff at 21:02:37; the guest stopped and the launcher exited at 21:02:39. |
| Returning home | Reopening from GNOME app search showed [Omarchy is ready](first-run-candidate2/returning-home.png), Launch Omarchy, Settings, Backup and recovery, Close, and the overflow menu. The complete storage explanation was reachable by [scrolling](first-run-candidate2/home-scrolled.png). |
| Returning boot | Launch started QEMU at 21:03:34 and reached desktop readiness at 21:03:46, about 12 seconds. The [desktop rendered again](first-run-candidate2/second-desktop.png), without downloading, repeating consent, or rewriting the first-session tip marker. |
| Final stop | A second normal shutdown completed at 21:05:05. [Final state](first-run-candidate2/final-state.txt) has no running Flatpak instances and an empty QEMU error log. |

The viewer did not reliably forward synthetic keyboard events from omabox.
Keyboard checks therefore sent RFB key events from inside `cleanview` through
the same test VM's VNC connection. They exercised Ubuntu's input path and the
installed QEMU window, rather than invoking guest shortcuts through SSH.
Mouse actions and screenshots used the existing isolated viewer.

The small launcher window puts longer details in a scrollable area above
the action buttons. The initial cramped appearance did not establish a
layout defect; the storage text remained accessible by scrolling.

## Validation and continuation state

The focused Go tests for home, quick setup, tips, clipboard consent, experience
and setup state passed in the devbox Go 1.27.1 container; see the
[test record](first-run-candidate2/focused-tests.txt). The
[launcher log](first-run-candidate2/shell.log) covers both boots and shutdowns.

The Ubuntu test VM and inherited viewer remain available for further work;
the nested Omarchy guest and launcher are stopped. The source fix was already
committed before this continuation. Only this report and its evidence were
added, without committing, pushing, publishing, or changing the test candidate.

This run did not repeat the earlier Software installation, test personal-account
setup, prove clipboard contents transfer, perform update/uninstall/reinstall,
or validate a physical GPU, audio device or another host desktop. Those checks
must not be inferred from this first-run result.
