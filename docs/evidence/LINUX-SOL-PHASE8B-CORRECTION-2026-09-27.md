# Phase 8B correction for Astra review

Status: local correction checkpoint. Findings 1 through 3 are fixed and the
specified isolated workflows passed with the final bundle. This is not public
release acceptance. No commit, push, tag, deployment or publication occurred.

## Exact review pair

- Starting checkout: `/home/bts/Projects/try-omarchy-linux-core`, branch
  `linux-core`, HEAD `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`, with the
  inherited Phase 3 through 8B uncommitted source. The correction source is
  preserved in `/data/try-omarchy-linux-spike/sol-phase8b-correction/source-delta.tar.gz`
  and described by the adjacent `source-delta.json`. Reconstruct it from the
  original 685-file baseline, Phase 8A delta, Phase 8B delta, then this delta.
- Final Flatpak:
  `/data/try-omarchy-linux-spike/sol-phase8b-correction/phase8b-correction-final.flatpak`,
  SHA256 `e14b7ff8053ff0653bcf7b5d76099514d56356d17fc93b0317adf4776dc6ca72`.
  Built on `devbox` from the correction source. The build log is
  `/workspace/try-omarchy-linux/phase8b-correction-build-r3.log`; the bundle
  was copied back and its local hash matched. User Flatpak installation reported
  commit `a5ed2f86be9972ec60b78e48b72fda8607f31073c0ef89edcb8427a91179f765`.
- Private guest revision 39 was unchanged:
  `/data/try-omarchy-linux-spike/sol-phase8b/guest-r3/SHA256SUMS`, SHA256
  `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`.
  Its download was served only from task-local loopback port 18090.
- Intermediate bundles `8870ddd8...` and `8542bca2...` were used to diagnose
  portal storage and boot timing. Evidence below refers to the final bundle
  unless a paragraph says otherwise.

## Findings and correction evidence

| Astra finding | Before | After and regression coverage | Final isolated observation |
| --- | --- | --- | --- |
| 1. Boot Cancel hard-stops QEMU | Fake QMP trace was `qmp_capabilities`, `quit`. | Ordinary Cancel, helper exit and first interrupt request ACPI `system_powerdown` and keep supervision alive. Once guest userspace is ready, the watcher repeats ACPI until the guest exits. A separately repeated interrupt retains the deliberate force-stop path. `TestLinuxBootCancelRequestsCleanShutdown` and `TestLinuxBootCancelRetriesACPIAfterUserspaceReady` cover protocol and handoff. The helper button reads `Stop Omarchy`. | Returning disk Stop during boot: `00:55:57` request, `00:55:59 guest stopped (poweroff)` in private-return shell log. Fresh custom disk Stop during boot: `01:02:15` request, `01:02:17 guest stopped (poweroff)`. Killing only the task setup-helper PID during boot of a separate sentinel clone produced `01:13:47` request, `01:13:49 guest stopped (poweroff)`. Each app process exited; no correction QEMU remained. The sentinel clone subsequently booted to its bar and wallpaper and shut down through the GUI. |
| 2. Pending shutdown answer lost at handoff | Readiness or timeout discarded the boot waiter's local pending answer. | One `linuxShutdownConfirmation` owns the pending channel through boot wait and watcher. It is cancelled on process exit. `TestLinuxShutdownAnswerSurvivesDesktopTransition` covers Keep running and Shut down across both ready and timeout; `TestLinuxShutdownPromptCancelledOnProcessExit` covers cleanup. | In the returning disk, the QEMU close dialog opened during boot and remained open after `guest desktop announced ready` at `00:56:53`. Keep running dismissed it and left the desktop alive. A second close and Shut down led to poweroff. In the final personal-account run, the desktop timer expired at `01:08:09` while the guest setup prompt was waiting; setup then finished, the desktop appeared at `01:09:22`, and a close confirmation after timeout still shut down with `poweroff` at `01:10:58`. The live timeout run exercised a post-timeout dialog, while the deterministic test covers a dialog already pending at the timeout boundary. |
| 3. Fresh preboot share Save failed | `EvalSymlinks` failed on the absent VM data directory. | Path validation canonicalizes the deepest existing ancestor, retains data/ancestor and symlink-alias rejection, and rechecks against a custom storage choice. `TestLinuxSharingBeforeDataDirectoryExists`, `TestLinuxFreshSettingsShareBeforeStorageChoice`, `TestLinuxFreshSettingsCancelKeepsSharingDisabled`, and `TestLinuxFreshSettingsRejectStorageInsideSavedShare` cover these cases. | In a new private XDG home with no data directory, the first action was Settings, portal folder choice, then Save. `settings.json` stored the document-grant path while `vm/disk.raw` remained absent. A custom portal location was selected next; copied settings retained the share. On restart, Settings still showed sharing enabled. The guest opened `/mnt/host`, saw `host-to-guest.txt`, and a folder created there appeared on the host. In a separate fresh private home, selecting a folder then cancelling Settings left both `settings.json` and the disk absent. |

The private portal runtime is nested at
`/run/user/1000/omabox/try-omarchy-correction/private-runtime-r3/doc/...`.
QEMU's default OFD lock failed on the earlier intermediate bundle at this
document mount. The correction detects this nested document-grant form and
sets `file.locking=off` only for portal-backed disks. The final QEMU command
used this option for the custom location, and the same disk booted and resumed.
`TestLinuxOnlyPortalDisksDisableQemuLocking` checks portal and ordinary paths.
The private chooser ran under a task-local D-Bus session and document portal;
the no-session-bus omabox runs are not counted as portal evidence.

The helper window's X close path was also repeated with this exact bundle,
separately from the Stop button. Closing it during boot of a new default disk
requested shutdown at `01:51:46` and QEMU exited with `poweroff` at `01:51:49`.
The same disk booted again to a rendered bar and wallpaper and shut down at
`01:53:23`. Closing the helper during boot of a separate returning sentinel
clone requested shutdown at `01:49:02` and exited with `poweroff` at
`01:49:06`; a subsequent boot reached the desktop and shut down at
`01:54:55`. Its sentinel hash remained
`a0140892991281ce57c0af6d59fb7c9c9645f103af48dcaedbf57d6bc017d9fa`.
Screenshots include `r3-fresh-close-progress3.png`,
`r3-fresh-after-helper-close-desktop.png`,
`r3-return-helper-before-close.png`, and
`r3-return-after-helper-close-desktop.png`.

The final personal path used disposable credentials. It reached the guest's
interactive username, password, keyboard and timezone prompts, completed
account setup, rendered the Omarchy bar and desktop, and mounted the portal
share. Screenshots include `r3-fresh-personal-setup.png`,
`r3-fresh-personal-correct-review2.png`,
`r3-fresh-personal-desktop-timeout.png`, and
`r3-fresh-personal-share-host.png` under the correction evidence directory.

The actual sentinel fixture was cloned from
`/data/try-omarchy-linux-spike/sol-phase8b/private-final`, not the earlier
`private-data` clone. Read-only `debugfs` on the stopped disk returned
`a0140892991281ce57c0af6d59fb7c9c9645f103af48dcaedbf57d6bc017d9fa`
for `/home/omarchy/Documents/phase8b-sentinel.txt` before helper failure,
after clean shutdown, and after the successful subsequent boot and shutdown.

## Checks

- `go test -race -count=1 -skip 'TestSavedSessionRAMSurvivesNewQEMUProcess|TestSavedSessionPublishesMatchedDiskAndRAM' ./...` passed inside a throwaway omabox: app `30.543s`, sign-update `1.020s`. The two saved-session tests remain excluded because the host QEMU 8.2.2 runtime does not satisfy their newer QEMU requirements; this is the inherited Phase 8B test gap, not a full-suite pass.
- Linux `go vet ./...` passed. Windows `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`, `go test -c`, and `go vet -unsafeptr=false ./...` passed. Native Windows tests were not run. The Windows unsafe-pointer vet suppression is the existing project gate for Win32 interop.
- `git diff --check` passed. Final Flatpak bundle hash was verified after copy.
- No correction VM, private launcher or task helper process remained after the runs. The unrelated Windows VM and unrelated omabox were not touched. The task omabox and local private evidence were retained for review.

## Remaining acceptance

| Item | Status |
| --- | --- |
| Personal setup timeout wording | Observed: after an intentionally slow interactive setup exceeded five minutes, the helper continued to show `Omarchy could not start` even after the guest desktop appeared. The guest kept running and shut down cleanly. The timeout message needs a live state update before release. |
| Pending prompt across a real five-minute timeout boundary | Deterministic Keep and Shut tests pass; a live prompt opened before the exact timeout boundary was not staged. The final live run checked confirmation after timeout. |
| Pre-userspace physical ACPI retry | The protocol regression covers retry after userspace readiness. Final live Cancel and helper-loss runs occurred after userspace announcement and before desktop readiness. |
| Distribution and updater | The pinned guest remains a loopback fixture. The existing-disk guest update remains pending commit/rollback work in Phase 10. No public release artifact was produced. |
| Broader parity | GNOME/KDE/X11, accessibility, fractional scaling, keyboard, external grants, clipboard, camera, battery, and Phase 9 controls remain open. No owner physical test was requested or inferred from omabox results. |

These correction checks are green for starting a local Phase 9 feature batch.
They do not close the release acceptance matrix.
