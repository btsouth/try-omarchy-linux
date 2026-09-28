# Astra review of Sol Phase 8A and 8B

Verdict: continue local development, but fix the three findings below before
starting the next Phase 9 feature batch. Phase 8B is an implementation checkpoint
with useful evidence, not accepted as complete. No publication is authorized.

## Review scope and evidence

Reviewed the Phase 8A and 8B reports, their code changes, the new guest patch,
retained logs and selected screenshots. Reconstructed the source from the
original 685-file baseline plus both Sol deltas; implementation files match the
live worktree. The archive metadata file `source-delta.json` is not product code.

Verified the reported artifact hashes:

- Flatpak: `713aad5c7f2bdd7da6875bdc9eb720c44939007a454c9acb56386c899e4e8cc8`
- Phase 8B delta: `286dd3036f5571194b9c3d05c09146f62c9a00191f915819a3943d2f7209139c`

Independent checks used a separate source copy on devbox at
`/workspace/try-omarchy-linux/astra-review-phase8b/app`. No implementation changes
were made to Sol's worktree. Review probes are stored locally at
`/data/try-omarchy-linux-spike/astra-review-phase8b/astra_review_test.go`.

Independent results:

- Existing `TestLinux*` tests passed under the race detector.
- Four guest `test_desktop_ready.py` cases passed. They stub the desktop tools;
  they do not contact the real desktop session.
- Three additional review probes failed deterministically, reproducing the
  findings below. The boot-cancel probe uses a fake QMP process and no VM/GUI.
- `git diff --check` passed before adding this report.
- Windows vet, build and test compilation passed independently in the isolated
  review copy. Native Windows tests were not run.

No new native GUI session or full VM boot was run for this review. Visual
assessment uses Sol's retained screenshots. Sol's exact-final first-run and
returning-disk boots are supporting evidence, not independently repeated here.

## Finding 1: P1, boot Cancel hard-stops a running guest

Location: `app/supervise_linux.go:93`, with the setup close/cancel handler in
`linux-ui/main.go`.

Phase 8B keeps the setup helper alive after QMP is ready, until the desktop-ready
message or five-minute timeout. During that interval, pressing Cancel, closing
the helper or an unexpected helper exit cancels `setupContext`. The new
`linuxDesktopCancelled` branch immediately sends QMP `quit`, with a process-kill
fallback. It does not first request ACPI shutdown or wait for a clean poweroff.

This extends the old short startup cancellation behavior into a period where
the writable guest is booted and can be provisioning, migrating configuration
or running applications. It risks losing in-flight writes. The review did not
demonstrate disk corruption; it demonstrated an immediate hard stop.

Reproduction: `TestAstraBootCancelRequestsCleanShutdown` starts a fake QMP
process, completes the capability handshake, then requests setup cancellation
while desktop readiness is pending. Recorded commands are exactly:

```text
qmp_capabilities
quit
```

Required fix: once QEMU is running, route ordinary cancellation/helper loss
through a clean shutdown lifecycle. Keep supervision alive until the process
exits, and distinguish a separately deliberate force-stop action from ordinary
Cancel. Preserve cancellation without disk creation during the earlier setup
stages. Make the UI wording match the action.

Acceptance: cancel and close the helper during boot on both a fresh and existing
disposable disk, and exercise helper failure. Verify ACPI/guest shutdown before
process exit, no orphan, retained sentinel, and successful subsequent boot.
Keep a focused protocol regression test. Do not use a real user disk.

## Finding 2: P2, desktop handoff drops a pending shutdown answer

Location: `app/desktop_ready_linux.go:33` and `:46`, followed by the new local
confirmation variable in `app/supervise_linux.go:212`.

During boot, close the VM window so the confirmation helper opens. If the guest
announces desktop-ready before the user answers, `waitLinuxDesktopReady` returns
immediately. Its local confirmation channel is lost. `watchLinux` starts with a
different nil confirmation channel, so clicking Shut down in the already-open
dialog has no effect. The timeout transition has the same ownership problem.

Reproduction: `TestAstraPendingShutdownSurvivesDesktopHandoff` queues a close
event, makes readiness true while the confirmation is pending, and observes
`linuxDesktopReady` returned before the answer. A later confirmed answer has no
consumer in the running supervisor.

Required fix: own confirmation state across boot, ready and timeout transitions,
or finish the outstanding decision before handing off. Cancel/close helpers
when their owning lifecycle ends. Avoid fixing this by suppressing readiness
forever or allowing duplicate shutdown dialogs.

Acceptance: open confirmation just before readiness and just before timeout;
both Keep running and Shut down must work exactly once after the transition.
Also check process exit while a confirmation is open. Include deterministic tests
and one isolated real-GUI reproduction after the fix.

## Finding 3: P2, fresh pre-boot Settings cannot save a shared folder

Location: `app/settings_linux.go:72` and
`app/main_linux.go:334` (`validateLinuxSharedFolder`).

The new home intentionally does not create the VM data directory. Open Settings
on a genuinely fresh install, choose an existing shared folder and save before
any earlier preference save has created that directory. Validation calls
`EvalSymlinks(dataDir)`, which fails because the data directory does not exist.
The otherwise valid shared-folder choice cannot be saved. Saving resources first
happens to create the directory and masks the problem.

Reproduction: `TestAstraFreshPrebootCanSaveSharedFolder` uses an existing exchange
folder and an absent new-install data path, matching the state passed by the
Settings flow. Validation fails with `lstat .../new-install: no such file or
directory`.

Required fix: support validation against the intended not-yet-created data path
without losing the protections against sharing the VM data or its ancestors.
Preserve symlink/portal alias safety and the read-only idle-home behavior.

Acceptance: first action Settings, then select a shared folder and Save, with no
preceding memory-only Save. Follow with storage/account setup and restart. Repeat
for a custom data location and verify cancellation does not enable sharing.

## What is good and should be retained

- The pre-boot home and real resource controls are a useful improvement over
  typed internal values. The examined light Settings and dark home screenshots
  support the claimed UI changes.
- Linux guest selection is separate from Windows defaults and keeps an explicit
  checksum pin. The loopback dependency is plainly disclosed.
- Desktop readiness distinguishes QMP and userspace from guest shell surfaces.
  The retained final handoff image shows a rendered bar and wallpaper as the
  helper transitions away. That is stronger evidence than SSH alone.
- Reports distinguish previous bundles from the final one and explicitly retain
  untested desktop, portal, accessibility and physical acceptance gates.
- Guest patch and source-delta recovery information are concrete and usable.

## Existing gaps, not additional new regressions

1. The hardcoded loopback release remains a private fixture. No-flag fixture
   success is not ordinary user distribution readiness. Keep this explicit.
2. The existing-disk evidence proves migration and sentinel preservation, not a
   completed Linux update transaction. Its retained `payload-update-state.json`
   still says `guestPending: true`, and `guest.previous` occupies about 6.1 GiB.
   Linux does not call the shared pending-payload commit/rollback flow. This is
   inherited Phase 10 work; do not describe it as a fully accepted updater.
   Address it before expanding guest-update/recovery claims.
3. Personal-account provisioning, timeout and boot-error UI, live revoked grants,
   and exact-final GNOME/KDE/X11 were not established by the final instant-account
   omabox screenshots. In particular, test what the user sees while the guest
   account setup requires interaction, not only the automatic trial path.
4. Sol's full-suite attempt failed two saved-session tests on host QEMU 8.2.2.
   The skipped-suite result is honestly reported but is not a full suite pass.
   Use a compatible isolated QEMU through `QEMU_SYSTEM`/`QEMU_IMG` for those tests
   before final hardening. Do not change tests to hide the runtime mismatch.
5. Broader Phase 9 controls, GNOME clipboard, external grants/drops, camera,
   battery and Phase 10 recovery remain outside this checkpoint.

## Instructions to send back to Sol

Fix findings 1 through 3 as a bounded Phase 8B correction pass. Use the review
probes as starting reproductions, add durable regression coverage in the product
tests, and report the exact new source/artifact pair. The probes currently live
only in the review copy and evidence directory, not the product test suite.

Rerun no-flag fresh and returning boot, personal-account interaction, boot
cancel/helper failure, pending shutdown across ready/timeout, fresh pre-boot
sharing and clean exit. Preserve the existing desktop-ready check and sentinels.
Use the private portal harness for portal behavior; the no-session-bus omabox
harness cannot prove it.

Return an updated report with before/after evidence and unresolved acceptance
rows. Continue Phase 9 once this correction pass is green, carrying forward the
remaining matrix and recovery gates. Keep all work local, keep the desktop
isolation rules, and do not request deferred physical testing yet.

## Review artifacts and cleanup

Local evidence: `/data/try-omarchy-linux-spike/astra-review-phase8b/`.
Remote source/probes/logs: `/workspace/try-omarchy-linux/astra-review-phase8b/`.
Files include `astra_review_test.go`, `astra-regressions.log`,
`linux-focused.log` and an app-source snapshot.

The fake QMP process exited during its test. No GUI/VM/server was started by
this review. Sol's implementation and retained disks/artifacts were not changed.
Only this review report was added to the product worktree. The separate main
checkout has concurrent dirty changes observed at the end of review; none were
made or modified by this review. Nothing was published.
