# Astra review: Phase 8B correction and Phase 9 slice

Reviewed 2026-09-27. Verdict: the three prior Phase 8B findings are closed for
this local checkpoint. Continue private development, but fix the new microphone
Save retry defect before accepting that Phase 9 control. This is not a public
release candidate or complete Phase 9 acceptance.

## New finding

### P2: retry reports a saved microphone choice without persisting it

Location: `app/settings_linux.go:89-91`.

The settings loop updates `desktop.MicrophoneDisabled` before calling
`saveDesktopPreferences`. If the write fails, the loop retains the changed
in-memory value. On the next Save with the same checkbox state, its comparison
decides nothing changed and skips the write. The function reports success while
the persisted preference remains unchanged. In the reproduced disable case,
the next launch still permits microphone input even though Save reported success.

The isolated reproducer starts with microphone access enabled, submits the
disabled choice, forces the first atomic file replacement to fail, restores the
original writable preference file, then submits the same disabled choice again.
It receives `Settings saved. They will apply when you launch Omarchy.` while
`microphoneDisabled` is still `false` on disk. This checks persistence, not actual
microphone capture. The normal save/restart test passes because it never fails a
write.

Only update the cached preference after persistence succeeds, or reliably save
the desired preference on every retry. Add regression coverage for failure then
retry in both directions while preserving unrelated camera/update preferences.
The error must remain visible until the requested state is actually saved.

Reproducer and failing race output:

- `/data/try-omarchy-linux-spike/astra-review-phase9/settings_retry_review_test.go`
- `/data/try-omarchy-linux-spike/astra-review-phase9/focused-race-final.log`

The test is outside the product tree. No implementation was changed by this
review. It speaks the setup helper's pipe protocol without loading GTK.

## Prior findings

1. Setup Stop/helper cancellation now requests ACPI shutdown and waits for the
   process. The added early-userspace retry remains a clean shutdown request.
   The focused fake-QMP tests passed, including boot cancellation and retry.
2. One shutdown-confirmation object owns the pending answer across both desktop
   readiness and timeout. Its cancellation follows process exit. The transition
   and process-exit tests passed.
3. Shared-folder validation supports the not-yet-created data directory and
   validates the eventual custom location. Fresh Save, Cancel, custom storage,
   unsafe containment rejection and future-directory tests passed.

Sol's correction report additionally records packaged live Stop, helper-close,
helper-failure, personal-account, portal and disk-sentinel checks. Those are
retained evidence from Sol, not live VM checks rerun by this review. The correction
report's own limits remain: the earliest shutdown retry and exact timeout handoff
have deterministic coverage rather than every possible live timing combination.

## Independent validation

- Reconstructed the baseline plus Phase 8, Phase 8B, correction and Phase 9
  deltas. All 700 reconstructed files matched the checkout before review-document
  additions. Result: `astra-review-phase9/source-check.json` under the data root.
- Verified both supplied Flatpak hashes and both new source-delta hashes below.
- Compiled a race-enabled test binary from an isolated source copy on devbox,
  using the existing pinned Go toolchain. Ran focused tests in a throwaway,
  network-isolated omabox with no session-bus connection or host audio.
- All selected existing Phase 8B/Phase 9 regression tests passed. The new
  microphone retry test failed as described above. The combined run therefore
  correctly exits with failure; it is not reported as a green suite.
- Linux `go vet ./...`, Windows `go vet -unsafeptr=false ./...`, Windows launcher
  cross-build and Windows test compilation passed on the isolated devbox copy.
  Native Windows tests were not run.
- Inspected Sol's retained home and settings screenshots. Basic identity is
  present and the new controls and shared-folder section are visible. This is
  not independent accessibility, desktop-matrix or complete visual acceptance.
- Late-desktop timeout dismissal passed its deterministic test. The fixed
  behavior still needs the live delayed-setup check called out in Sol's report.

The full suite was not rerun here. Sol's broader race run excludes two saved-
session tests because of host QEMU 8.2. That limitation remains open. No new live
VM, physical microphone, desktop portal or real desktop test was performed here.

The first omabox startup hit an unrelated orphan Go-cache cleanup permission
failure. Using a task-specific `XDG_CACHE_HOME` allowed the isolated run without
changing other boxes. An initial reproducer assertion expected different errno
wording; the final test accepts the observed replacement error and demonstrates
the actual persistence failure. Use `focused-race-final.log` for the result.

## Artifact identity

Starting local HEAD: `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`, branch
`linux-core`. These snapshots include uncommitted work.

| Artifact under `/data/try-omarchy-linux-spike/` | SHA256 |
| --- | --- |
| `sol-phase8b-correction/phase8b-correction-final.flatpak` | `e14b7ff8053ff0653bcf7b5d76099514d56356d17fc93b0317adf4776dc6ca72` |
| `sol-phase8b-correction/source-delta.tar.gz` | `246197f3283a2aa610ee6cac4c85b45d1b62f25913d5e50df456eb0e2bf18c9b` |
| `sol-phase9-slice/phase9-slice-final.flatpak` | `158d8f3d08fdedb13839cde0158da0ddff89f70b24b9b5ef48646570bc19b969` |
| `sol-phase9-slice/source-delta.tar.gz` | `24d25f585ae83aa136a1e6749b0de235a3c8f28ce4ff16b4815efcdd14dd4e37` |

The guest remains Sol's private revision 39. Neither a valid source chain nor a
matching Flatpak hash establishes public distribution or release acceptance.

## Continue toward release

Use [Linux release gates](../LINUX-RELEASE-GATES.md) as the next work brief.
Close this finding and the live late-desktop check first, then complete everyday
controls and file workflows. Installation, branding, uninstall/data retention,
updates and recovery are required product work, followed by exact-candidate
hardening and the separately authorized physical pass.

No application-code changes, commits, pushes, PRs, publications or real-session
actions were made during this review. Only local review/planning documents and
isolated review fixtures were added or updated.
