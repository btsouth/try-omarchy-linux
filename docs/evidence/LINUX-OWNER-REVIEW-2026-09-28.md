# Private owner review VM

The owner requested a VM review before public release and has no other test
hardware available. A dedicated Ubuntu 24.04 VM is prepared on devbox.

## Candidate and acceptance

Installed through Ubuntu Software from the candidate 4 flatpakref. Exact commit:
`45218ae8e5ddd8342c3ddcd9d0622e42152cbe6654ccf706edde89a4281eb027`.

A fresh account had no Flatpak app or Omarchy guest cache. Software installed
the app and runtime. Application-menu launch, Try it now, real guest download,
verification, unpacking, disk preparation, clipboard explanation, GNOME grant,
shortcut inhibition, visible desktop, terminal and keyboard release passed.
`whoami` returned `omarchy`. Normal shutdown completed in three seconds.

Guest log clock: setup started around 02:07 UTC September 29, desktop ready
02:09:30, shutdown requested 02:10:26, stopped 02:10:29. Evidence is in
[owner-review-20260928](owner-review-20260928). This completes the fresh trial
path on candidate 4 itself, extending the earlier candidate 3 full journey and
candidate 4 update/refusal regression checks.

## Review environment

- Host: devbox; 4 CPUs, 8 GB RAM, KVM with nested virtualization.
- Disk: `/var/tmp/try-omarchy-owner-review/desktop.qcow2`.
- Launch: `bash /var/tmp/try-omarchy-owner-review/launch.sh` on devbox.
- VNC: devbox loopback 5923. Maintenance SSH: loopback 22262.
- Ubuntu account: `ana`, a disposable fixture account.
- Local connection command: `~/.local/share/try-omarchy-review/connect.sh`.
- VNC uses an SSH tunnel, not a publicly exposed listener.
- Viewer uses Ctrl+Shift+Alt modifiers, avoiding the app's Ctrl+Alt shortcuts.

Snapshots: `before-install` has Ubuntu Flatpak prerequisites and the downloaded
installer, with no installed app or VM data. `verified-first-boot` preserves
the passing first-run test. The owner receives the restored before-install
state, with Software showing the candidate ready to install.

The package repository is served privately inside this test VM at the URL
embedded in the signed artifact. The installer is predownloaded in Downloads.
Public GitHub publication is not simulated as a live release. This review
starts at installation, after Ubuntu prerequisites and the download.

The prepared Ubuntu snapshot had a redundant networkd wait service despite
NetworkManager handling its interface, causing a two-minute boot delay. It is
disabled in the review VM. No host desktop or security policy was changed.

## Scope

Ready for owner review of the first-user VM experience. This is not physical
hardware acceptance or a published release. Exact preview 1 migration and live
GitHub delivery remain separate release checks. Do not reset this VM after
handoff without the owner's instruction; their review work will be on it.

## Owner findings

- The owner approved the README GIF and confirmed the light and dark home and
  Settings screens.
- Settings looked cramped, with its last line of text sitting on the first
  button. The window now leaves space above the buttons, shows a divider when
  the page scrolls, and gives Settings bold group headings, dimmed help text
  and full-width wrapping. Rechecked in light and dark at the default size.
- During an Omarchy update inside the guest, Enter opened terminals behind the
  update prompt. Reproduced in the agent VM: with TigerVNC fullscreen under an
  Omarchy host, a host-caught Super shortcut sends Super down to the review VM,
  the host menu takes focus and receives Super up, and TigerVNC never sends the
  release. Ubuntu keeps Super held, so Enter reaches the guest as Super+Enter.
  One Super tap clears it. The app releases held keys correctly when another
  Ubuntu window takes focus, so this is a review-viewer artifact, not an app
  defect.
