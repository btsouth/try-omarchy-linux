# Linux Phase 8A review checkpoint, 2026-09-26

Private local work on `linux-core`. This checkpoint implements and inspects the
returning-user launcher. It does not complete Phase 8B, everyday parity,
installation/recovery, or exact-candidate hardening. Nothing was pushed,
committed, published, or sent to an external service.

## Source and recovery

Starting HEAD: `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`. The inherited
Phase 3 through 7 tree was dirty, including most Linux files as untracked.
The 685-file baseline is
`/data/try-omarchy-linux-spike/sol-handoff-2026-09-26/source-baseline.tar.gz`
with SHA256 `7577679b68d44c0cdb786e35626652bcfc44a205b3490e02af6bf6bc284a1540`.
The separate Sol delta, including untracked additions and this report, is
`/data/try-omarchy-linux-spike/sol-phase8/source-delta.tar.gz` with a file/hash
manifest at `source-delta.json` in the same folder. Do not extract it over the
worktree without reviewing paths. No product commit was made.

Sol implementation files, compared by hash with the inherited baseline:

- `app/main_linux.go`, `app/first_run_linux.go`, `app/settings_linux.go`,
  `app/setup_window_linux.go`
- `app/launcher_home_linux_test.go`, `app/setup_window_linux_test.go`
- `linux-ui/main.go`, `linux-ui/README.md`
- `docs/LINUX-PLAN.md`, `docs/LINUX-PARITY.md`, `docs/LINUX-ACCEPTANCE.md`,
  this report

The main Windows checkout and guest patch builder were left unchanged. The
guest builder remains at local commit `d5a57b17ca8a58f02257d08ad49cc95960e49ab4`.

## Behavior and parity state

| Area | Current result | Evidence level / remaining work |
| --- | --- | --- |
| Phase 8A home | Ordinary desktop launch waits for **Launch Omarchy**. It shows saved disk status, storage path, Settings, and About/help. Closing it exits without disk creation or VM startup. Explicit CLI flags still start directly; `-launcher` can opt back into the home. | Implemented, focused tests and isolated visual checks. |
| Phase 8A settings | Settings open before boot. Automatic or bounded manual RAM/CPU, rendering choices, shared-folder choice, and unsaved resource-default reset replace technical text fields. Save preserves unrelated settings and disabled-share intent. | Automated save/cancel tests; isolated manual save/reopen/cancel and keyboard checks. Running-VM restart feedback is automated-tested, not exercised with a live guest on this bundle. |
| New install choice | Preferences saved before installation no longer claim the default storage path; a selected location receives those preferences. | Automated test. The exact new-install portal chooser was not rerun in a live sandbox at this checkpoint. |
| Phase 8B | First-run completion, guest-ready feedback, actionable errors, metadata, and Linux guest artifact selection | Open. Normal GUI launch still inherits the Windows v0.4.0 guest default. |
| Phase 9 | Audio/mic GUI, external grants/drops, camera, battery, display/input/network controls and platform scope | Open. GNOME automatic clipboard remains unsupported; host keyboard-group changes are not tracked. |
| Phase 10 and 11 | Linux update/recovery policy and exact-candidate matrix | Open. Prior runtime matrix is useful inherited evidence, not acceptance of this bundle. |

Physical laptop, real speakers/microphone/camera, suspend, gestures, and actual
monitor behavior remain unverified by request. Linux host-app, USB and LAN scope
need explicit product decisions as Phase 9 work proceeds.

## Artifact identities and matrix boundary

| Item | Identity |
| --- | --- |
| Review Flatpak | `/data/try-omarchy-linux-spike/sol-phase8/com.tryomarchy.TryOmarchy.flatpak`, SHA256 `ffc68a90bffc2c1faeda0449f2aea2dab2f8dd8ce7cb1cb48a04bb0bdc1b3587` |
| Installed app OSTree commit | `61cae7f24049858d82ffaa8318b471c9946d047ce4290056174ebd8855ff00b0` for local `bts` |
| Packaged launcher | SHA256 `4580da99e6ca8ec43b6559a609293b897de262c4abaf130fa946f7f27c0cfd39`; cgo-free source-check binary SHA256 `455ad9d69605785d31140896fcae7f55f244a83c6d7c664cbc89f78bff38b023` |
| Packaged GTK helper | SHA256 `18279243ed680ff2da5068aca76c0576a06a04045dc8543adaf76deb0624dabe` |
| Runtime | GNOME 50, QEMU 11.1.1, virglrenderer 1.3.0; Flatpak 1.18.3 locally, 1.14.6 on devbox |
| Private guest | `guest-manifest.json` SHA256 `5107ae69ace4ecbd08ee77f25b0a4045cb2deee73021319186f3ac69bcb4a800`; `SHA256SUMS` SHA256 `9bf24910f99030944abfaf552af237df35a7f93d6df0b989af2373acd4cca8ea`; kernel 7.2.7-arch1-1, compatibility 37, builder `d5a57b1` |
| Hosts | Local Omarchy kernel 7.2.5-4; devbox Ubuntu kernel 6.8.0-139; isolated omabox Hyprland, 960x720 and 800x480 |

The Phase 8A GTK rows in this report used the review bundle above, except
screenshots explicitly labeled as the earlier layout-fix pass. No guest was
booted on that review bundle yet. The inherited Phase 3 through 7 Intel,
NVIDIA omabox and X11 checks used bundle
`beae8a169e485363f02546e7e2179917a2c4a0d72c83ee3ce59653eaf75f4590`
with OSTree commit `85aa3ab33c9c5822fb0dc52a0428bf8081a75f07ea8b0060d22c66b8a5051d69`.
Inherited GNOME and KDE checks used its preceding bundle; they were never
claimed as checks of the exact later bundle. The private guest artifact pair
is unchanged, but the Phase 8A bundle was not paired with it in a running-guest
test at this checkpoint.

## Checks and visual evidence

Remote source copy: `/workspace/try-omarchy-linux/phase3-src`. Logs:
`/workspace/try-omarchy-linux/sol-checks-phase8a/`. The final Flatpak build
command was `runtime-build/linux/build-flatpak.sh
/workspace/try-omarchy-linux/phase3-flatpak`; its log is
`flatpak-build-review.log`. Local screenshots and harness logs are under
`/data/try-omarchy-linux-spike/sol-phase8/`.

- `go test -race -count=1 -skip
  'TestSavedSessionRAMSurvivesNewQEMUProcess|TestSavedSessionPublishesMatchedDiskAndRAM'
  ./...` passed in `linux-tests-exact.log`. The focused new launcher tests passed.
- Linux `go vet ./...`, Windows `GOOS=windows GOARCH=amd64 go vet
  -unsafeptr=false ./...`, Windows cross-build and Windows test compilation
  passed. Native Windows tests were not run. Logs: `linux-vet-exact.log`,
  `windows-vet-exact.log`. The main launcher cross-build remained cgo-free.
- The unmodified full race command was attempted. Its two saved-session tests
  failed because devbox host QEMU is 8.2.2 and rejects QMP `exit-on-error`;
  see `linux-tests.log`. This is an environment limit, not a pass claim.
- `git diff --check` passed locally. The standard remote `check.sh` first
  stopped at `go: command not found` in root's non-login PATH; equivalent
  checks then used the retained pinned Go toolchain. Remote source has no Git
  metadata, so its final `git diff --check` step was done in the local worktree.

Final-bundle screenshot set, all inside the task omabox with the host session
bus/audio denied: `screenshots/home-review.png`, `settings-review.png`,
`about-review.png`, `home-small-review.png`, `settings-small-review.png`,
`settings-small-bottom-review.png`, `home-dark-review.png`, and
`settings-dark-review.png`. The Save/reopen/cancel sequence used the immediately
preceding layout-fix bundle and a disposable path under `test-data/final-new`;
it stored 4096 MiB, 2 CPUs and software rendering. Cancel after Restore
defaults kept that file unchanged. Keyboard Tab then Enter opened Settings.
The final review bundle reopened and displayed those saved values. At 800x480,
the scrolled content and fixed actions remained reachable. These are simulated
screen and theme checks, not physical display acceptance.

Prior first-run screenshot: `/data/try-omarchy-linux-spike/phase3-flow/devbox-account-final.png`.
Prior running guest screenshot: `/data/try-omarchy-linux-spike/phase5/local-smoke-final/desktop.png`.
Prior synthetic progress and error screenshots:
`/data/try-omarchy-linux-spike/phase3-ui/progress-final.png` and
`/data/try-omarchy-linux-spike/phase3-ui/error-final.png`. Prior shutdown
confirmation screenshot: `/data/try-omarchy-linux-spike/phase3-flow/devbox-confirm.png`.
These are inherited checks on earlier artifacts, not review-bundle results.
The shutdown image proves only the visible confirmation prompt, not a completed
guest shutdown. QMP readiness is not guest desktop readiness.

Inherited existing-disk migration retained the Intel sentinel and saved shared
folder grant at compatibility 37. Inherited setup cancellation/resume,
close/dismiss/helper-failure and portal persistence checks are in
`docs/evidence/LINUX-INTEGRATION-2026-09-26.md`. None of these were rerun with
the Phase 8A bundle. The new GUI close test left its disposable directory
uncreated. A Flatpak `XDG_DATA_HOME` override did not move its default app data
path; that probe stopped at the read-only home. The real default app data
folder remained absent.

## Serious issues and next actions

| Issue | Impact and reproduction | Next action |
| --- | --- | --- |
| Wrong guest default on ordinary Linux launch | `app/manifest.go` still chooses the Windows v0.4.0 guest unless private `-release` and trusted checksum flags are supplied. The desktop entry does not supply them. | Phase 8B/10 Linux-specific private artifact selection; keep Windows defaults unchanged. |
| Helper handoff at QMP | QEMU can answer before Omarchy shell starts; the setup window can disappear before a usable desktop. Earlier first boots intermittently missed the shell start hook. | Phase 8B shell-ready/timeout/failure states, then repeated fresh boots in Phase 11. |
| External file permission path | Real sandbox permission acquisition and FileTransfer portal grant roundtrip have not passed live tests. | Phase 9 private portal and external-drop test, with visible transfer errors. |
| GNOME clipboard | No automatic data-control path on GNOME; a selected share is a fallback, not clipboard parity. | Investigate supported APIs and request an explicit product decision if none is viable. |
| Remaining parity and recovery | Live audio controls, camera, battery, keyboard-group changes, host-app/USB/LAN decisions, backup/reset/move/update and exact-matrix validation remain. | Work through Phases 9 to 11 and record each accepted difference. |

## Cleanup and next task

The task omabox and any exact test launcher/helper processes were stopped after
screenshots. No test QEMU, private Sway, desktop VM, loopback release server, or
new host audio/portal service was left running by this checkpoint. Logs,
screenshots, the verified Flatpak, source baseline and delta remain under
`/data/try-omarchy-linux-spike`. Devbox retained its offline build cache and
bundle. The local `bts` Flatpak installation was updated to the review commit;
no new tool/runtime package was installed. The product worktree remains dirty
by design, and the main checkout stays clean.

Next bounded task: Phase 8B, starting with private Linux guest selection and
guest-ready/timeout/failure handoff, then first-run and error-state UI. Physical
acceptance and any publication decision remain separate; nothing was published.
