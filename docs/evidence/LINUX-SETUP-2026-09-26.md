# Linux setup window, 2026-09-26

Uncommitted local candidate on `linux-core`, based on `e9d8ecd`. Nothing published.

The launcher now starts a separate GTK4/libadwaita helper before its preflight
checks. It shows status, determinate and indeterminate progress, errors, and
cancellation. Errors wait for dismissal. Cancelling setup retains existing VM
data. The helper closes when QEMU answers on its control socket; this does not
claim the guest desktop is ready. `-no-gui` retains terminal-only operation.

The helper has its own Go module and Flatpak build module. The launcher remains
cgo-free. GNOME 50 supplies GTK/libadwaita and the Freedesktop 25.08 graphics
extensions. All Go dependencies are pinned and downloaded as hashed sources
before the offline builds.

## Validation

- Linux `go test -race -count=1 ./...` and `go vet ./...` passed on devbox.
- Windows vet, launcher cross-build, and test compilation passed. Native
  Windows tests were not run; they remain a CI-only check.
- Pipe tests cover normal handoff, cancellation, helper failure, Unicode and
  multiline errors, dismissal, and progress updates with a slow reader.
- The Flatpak built in the GNOME 50 SDK on devbox. The final build retained
  build directories with `--keep-build-dirs` for further GTK development.
- In the local omabox, the helper rendered with the default Vulkan renderer
  on NVIDIA. Progress, indeterminate status, error wrapping, Cancel visibility,
  desktop identity, and dismissal were inspected. Both titlebar cancellation
  and keyboard activation of Cancel stopped downloads with exit code 0. A
  recovery sentinel remained unchanged. `-no-gui` opened no window and returned
  exit code 1 for a missing QEMU executable.
- The final helper also displayed a real missing-QEMU error from the launcher.
  Dismissing it returned exit code 1 and left no window or helper process.
- The final product Flatpak was installed for `bts` on devbox. A fresh setup
  downloaded, verified, unpacked, prepared a disk, and booted to the Omarchy
  desktop on the Intel host. The setup helper exited at handoff. The harness
  denied host audio and session-bus access; the expected silent-audio fallback
  succeeded. Window close requested shutdown at 13:50:37, and the guest
  powered off at 13:50:39 with launcher exit code 0.

## Artifacts

Local evidence and bundle: `/data/try-omarchy-linux-spike/phase3-ui/`.
The screenshots `download-final.png`, `preparing.png`, `error-final.png`,
`preflight-final.png`, `devbox-setup.png`, `devbox-desktop.png`, and
`devbox-stopped.png` record the
checks above. `error-final.png` uses controlled protocol input for Unicode
and multiline text; `download-final.png` and `devbox-setup.png` show real setup.

Bundle SHA256:
`29cd5994820a4ac41a4b1a17442467f7e4a7dd9899efc6717f136e397d765a19`

Packaged launcher SHA256:
`40e53a4d2d9015af10584c45367ea62b0b11b056d3b7ae7ce6c11e65f149c3f7`

Packaged helper SHA256:
`44aa78bf1d728ef619e10ad97463e198039edd2ffb45e350a4a4c6a9ae229911`

Devbox source snapshot: `/workspace/try-omarchy-linux/phase3-src/`.
Build state: `/workspace/try-omarchy-linux/phase3-flatpak/`.
Fresh test VM data: `/workspace/try-omarchy-linux/phase3-data/`.

## Status at this earlier checkpoint

Subsequent first-run and integration work is recorded in
[Linux integration evidence](LINUX-INTEGRATION-2026-09-26.md).

### Original remaining work

First-run choices (data location and account mode), shutdown confirmation,
and keeping the guest awake while visible are the next Phase 3 work. Clipboard,
file transfer, portal sharing, real audio, guest HiDPI, and GNOME/KDE/X11 VM
coverage remain separate work. No physical laptop or real desktop-session
tests were performed. The local installed product Flatpak was not replaced;
devbox now has this candidate instead of the spike Flatpak.
The task's test VM, local omabox, remote headless compositor, and loopback file
servers were stopped after validation. Test data and build artifacts remain.
