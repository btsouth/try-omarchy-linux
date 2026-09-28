# Linux release acceptance continuation

Acceptance started from commits `a9728ce` and `7c27ac3`, based on public
`25f3aed`. After reviewing the results, the owner authorized committing and
publishing as appropriate. The tested bundle is being prepared as a GitHub
prerelease for outside testing; stable acceptance and Flathub remain pending.

## Scope

The owner confirmed core VM use, files, recovery and desktop support as the first
release scope. Camera capture, live audio switching, gestures, LAN/bridge,
USB, host-app launching and host authentication are documented limitations.
Hardware testing must come from outside testers. The separate
[hardware checklist](../LINUX-HARDWARE-TESTING.md) is ready for that stage.

## Review corrections

- An explicit `-dir` launch could keep pointing at the original after moving it
  from the idle home. Both the home and launch resolution now follow the move
  record, without changing an unrelated default VM selection.
- Reset-disk cleanup deleted the disk before noticing other files in its folder.
  It now refuses unexpected contents before removal.
- A linked `vm` directory could make reset-disk cleanup delete an external disk.
  Cleanup now validates the path, and keeps the disk lock held through unlink.
- Recovery no longer reserves space for the large decorative icon; its shorter
  description fits at the normal window size. Reset cleanup explicitly says
  that it removes all listed retained disks.

The three new regression tests fail on `7c27ac3` and pass with these changes.
No shared Windows implementation changed.

## Automated checks

On devbox, inside the pinned GNOME 51 SDK with private D-Bus and no display:
Linux race suite, Linux vet, Windows vet, Windows cross-build/test compilation,
and GTK helper tests passed. QEMU was 11.1.1 and Go was 1.27.1. The Flatpak
manifest and AppStream linters both returned zero.

Native Windows CI passed on public `25f3aed`; that is baseline evidence, not a
CI run on these unpublished changes.

## Installed GNOME observations

The disposable desktop VM is
`/workspace/try-omarchy-linux/linux-r43-test/desktop.qcow2` on devbox. Its
preserved parent was not changed. GUI interaction used the existing `linux-r43`
omabox VNC viewer, never the owner's desktop.

- Opus's GNOME 51 package, OSTree `f3bb5f9764ea0ee6022abe67ffcf5bcdef6b48d63041ef80827cc953db712dd6`:
  cancelling a portal-folder move midway removed staging and kept the original
  24 GiB disk, inode `7340085`. Home and Close both worked afterwards.
- GNOME Software ordinary uninstall, with Keep selected, removed the user app
  and retained that disk unchanged. An older system package was then removed
  from this disposable VM to avoid confusing package selection.
- First corrected bundle SHA256
  `13cedf904266e73e63ee4a4620ff51ce6aedad668152137ec391faba6fb5e4c3`, OSTree
  `3c0cbcbbad2c3e4d029c948ed20d5dea45a3cbc0ec0f91b102921a61126abc6b`:
  graphical install succeeded, metadata showed the correct name/icon/developer
  and screenshot, GNOME search found the app, and an ordinary launch found the
  retained disk without a download. Recovery text and all available actions fit.

The local-bundle Software page initially showed no screenshot while parsing the
uninstalled file, then displayed the metadata after installation. This is a
bundle-install rehearsal, not acceptance of a published Flathub listing.

## Final candidate

Source is local HEAD `7c27ac3b6630b9adbbf4cf2ba3604849d948f91b` plus the
uncommitted app/linux-ui corrections. The source delta SHA256 is
`c94d0ffd9e2eba077ee46e45797890c28c0f5e9a9d1297725bdb4195dd0dfdbf`.
The build used the pinned SDK image
`ghcr.io/flathub-infra/flatpak-github-actions@sha256:78d969b18225ae107ca29497bf09c4a3f791e5f1838a9a912a529d0024fb6210`.

- Bundle: devbox `/workspace/try-omarchy-linux/linux-r43-test/artifacts/astra-final2.flatpak`
- SHA256: `d1ac02f6f5d74a1cee9b3db7bb8c29ec7bc8ca3970a3df3ea8286fed6d3fe382`
- Installed app OSTree: `1672445d13a2b6caf08380ea4a5bf70b82b9957625eaa277fceb3f7a7d48ef9a`
- Runtime: `org.gnome.Platform/x86_64/51`
- Guest: published `linux-v0.1.0`, compatibility 43, kernel `7.2.7-arch1-1`.

The final change also removes a misleading Cancel button during move activation
and verified-original cleanup, which cannot safely be cancelled. Those stages
now say to keep the window open. A GTK regression covers cancellation racing
with the transition. The installed final helper also rendered that state with
neither Cancel nor a close button, then restored the home controls. This was
a synthetic UI protocol check, separate from the real cleanup observation.
All automated checks above passed after this change.

## Move and package lifecycle

The corrected package completed a portal-selected move of the 24 GiB VM to
`/home/fresh10i/MovedVM/try-omarchy`. It retained the original, hid cleanup
until the moved guest had booted, and launched from the new location normally.
The original matched its recorded SHA256
`ee8dacb683bf04a762cc1d826ec3bda5dc40baf0d115ca57bb57e7665550f1b7`.
After boot, confirmed cleanup removed only the retained original. The moved
disk and redirect remained. Keep running and confirmed Shutdown both worked.

Those full move/cleanup observations used intermediate GNOME 51 packages
`13cedf90…` and `d8803bec…`. The final package updated that moved installation,
found it normally, and booted its saved file on KDE, X11 and GNOME. This is
an update of a stopped VM. A later final-candidate clean guest also stayed
running through replacement with `d8803bec…` and reinstallation of `d1ac02f6…`;
a new Documents file remained readable before shutdown.

## Desktop and file results

The host desktop VM uses Ubuntu 24.04 with GNOME 46, KDE Plasma 5.27.11 and
Xfce X11. It renders through software graphics in a disposable nested VM.
These results do not establish physical GPU or audio compatibility.

| Host session | Candidate | Observed result |
| --- | --- | --- |
| GNOME Wayland | Intermediate `d8803bec…` | Two-way Unicode text, PNG and external file copy/paste passed. The host file was unreadable directly in the sandbox; GNOME Files supplied the portal grant. |
| GNOME Wayland | Final `d1ac02f6…` | Ordinary retained-VM boot; denied clipboard consent showed a clear message and did not block the guest; restart offered consent again; granting it restored guest-to-host Unicode text. |
| KDE Wayland | Final `d1ac02f6…` | Normal home, visible guest desktop and two-way text/PNG/file clipboard passed. File automation used app-owned fixtures, not an external Dolphin grant. |
| Xfce X11 | Final `d1ac02f6…` | Normal home, visible guest desktop, two-way text/PNG/file clipboard and guest reboot passed. |
| Omarchy/Hyprland host | Not rerun on this final package | Earlier evidence remains historical. The isolated local omabox has no `/dev/kvm`; its viewer is not an app-on-Hyprland acceptance test. |

GNOME external-file SHA256 was
`1397bcc69c6eb13c884226b40d0a15ac179497dfdb926b3f37fa90242a11be67`.
The PNG matched in both directions:
`51ab9cdbe436375f510ed5b05fd7106c2e6518d6279bb1e043d4bd9e100692f5`.
The guest Documents sentinel, also pasted back into GNOME Downloads, matched
`c653c34e29698d8add08efac851fb6b3b9f412865c16db5014a285dd3071ed0f`.
That sentinel survived the final package update, desktop-session changes and
X11 guest reboot.

Switching the test VM from KDE to X11 left an inactive Wayland socket. Flatpak
then withheld its fallback X11 socket, so the first launch failed. Removing
that stale socket after checking it had no listener allowed the ordinary
package to run without permission overrides. This was a test-session artifact.
GNOME was restored after the matrix. No crash report was sent.

## Final-package recovery

Back up first completed through the normal portal picker, producing
`try-omarchy-backup-20260928-154426.495057739.zip` (6,612,122,938 bytes).
The reset retained the original 24 GiB disk inode `269039` in
`vm/before-reset-287841895/disk.raw` and created disk inode `269175`.
The clean guest booted visibly; its Documents sentinel was absent, as expected.
Settings, including the SSH choice, remained usable. Its newly generated SSH
host key was accepted only after the deliberate reset was confirmed.

Restore as a copy created
`AstraFinalRestore/try-omarchy-restored-20260928-155158.494351757` without
changing the current selection. Independent SHA256 of the restored disk matched
the archive manifest:
`e84b3699adfc3d22bb9501a6387f36ceeb4d6309a50af9d969b47afb44cf7c61`.
The backup manifest is retained with the evidence. Independent hashing also
confirmed the retained original had the same disk hash. Confirmed cleanup then
removed that retained disk and kept current disk inode `269175` unchanged.
Use existing data folder selected the restored copy through a new portal grant.
It booted to the visible desktop and the guest Documents sentinel kept SHA256
`c653c34e29698d8add08efac851fb6b3b9f412865c16db5014a285dd3071ed0f`.

A failure fixture then simulated an update which had started but was never
confirmed: with the VM stopped, its valid guest directory was retained as
`guest.previous`, an incomplete replacement directory was created, and the
pending-update record was written. An ordinary final-package launch restored
the previous directory, cleared the pending record, showed the rollback notice,
and booted the same Documents file with the same hash. This exercises recovery
from the persisted failure state, not an actual power cut during download.

Removing the restored folder's document export while stopped produced a clear
storage-unavailable home with Launch, Settings and Recovery disabled. No
replacement disk was created. Use existing data folder granted access again
and selected the same restored folder. The test guest and app were then stopped;
the desktop VM, archive, clean reset VM and restored copy remain available.

A second restore was forcibly terminated with `flatpak kill` after its staged
`vm/disk.raw` exceeded 1.6 GB. No completed destination was published. Reopening
the app retained the working restored VM selection; it booted visibly and the
Documents sentinel still had its original hash. Abrupt termination left the
incomplete `.try-omarchy-restore-3651067293` staging folder. It was removed
manually only after verifying the working copy, using that exact test path.
Automatic cleanup of arbitrary abandoned external restore folders is not claimed.

The final guest passed a bounded 90-second CPU and 512 MiB memory load alongside
eight 64 MiB write/fsync/read/hash cycles. The Documents sentinel remained
unchanged, and the disposable load file was removed. This is a software load
smoke test, not long-duration hardware stress or suspend/device acceptance.
The guest was shut down cleanly afterwards.

## Evidence and remaining gates

Logs and selected screenshots are in [linux-release-20260928](linux-release-20260928).
The regression baseline log records the three reproduced failures; final Linux
and GTK logs and `check-final2.log` record passing checks.

This is not a declaration that every release gate has passed. Remaining
acceptance includes exact-candidate Omarchy/Hyprland coverage, extended everyday use and
physical device recovery/removable-storage testing. The software evidence covers
move cancellation, forced termination during restore and injected unconfirmed
update recovery; it is not a claim to have cut power at every write boundary.
The earlier [v0.1.0 rehearsal](LINUX-V010-CANDIDATE-2026-09-28.md) already
proved an installed revision 39 to 43 guest upgrade, unconfirmed-update rollback
and successful retry. The guest/update implementation did not change here;
that evidence and the passing final automated rollback tests are retained,
without claiming a new cross-version migration on this final bundle.

Outside testers must supply physical GPU, audio/microphone, suspend/resume,
battery and monitor results. Camera, live audio routing, gestures, LAN/bridge,
USB, host-app launching and host authentication are accepted feature limits.
Flathub delivery, public-app publication and release acceptance remain separate.
Publication status is recorded in the repository README. No Flathub submission
was made by this run.

