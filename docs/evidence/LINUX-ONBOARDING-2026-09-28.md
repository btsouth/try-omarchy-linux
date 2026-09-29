# First-run onboarding improvements, September 28, 2026

## Baseline and scope

The published app remains `linux-app-v0.1.0-preview.1`, source
`159fbf3ffe4fba089e8b18d385e12067807962d0`. This work starts at
`9a77dd6ba270c5ad1a4c8b05726265255aa9fc30` on `linux-first-class`.
The 11 intervening commits add quick setup, storage guidance, progress and
retry, clipboard consent, recovery cleanup, keyboard release, first-session
help, and a signed Flatpak repository. The home action-button fix is included.

The preceding [candidate 2 journey](LINUX-FIRST-RUN-CANDIDATE2-2026-09-28.md)
passed trial boot, returning launch, keyboard release and shutdown. This
record covers the subsequent source changes and their validation. Neither
record establishes publication or physical-hardware acceptance.

## Changes

- First-run home states the download size, disk requirement and trial versus
  personal-account choices before setup. Detailed storage rows are expandable.
- Choice dialogs use the compact libadwaita layout to leave more space for
  their explanation and actions.
- Clipboard consent happens after download and disk preparation. Cancel at
  the explanation now cancels setup instead of proceeding to boot. Not now
  continues without sharing, as before.
- The decompression byte counter is atomic. The race detector found that
  zstd read-ahead and progress reporting accessed it concurrently.
- The install page explains Ubuntu Software and Flatpak prerequisites,
  installation, first launch, trial credentials and keyboard controls. It
  includes a terminal fallback and preserves update/data-removal guidance.

## Automated and browser checks

On devbox, with Go 1.27.1:

- Full Linux `go test -race ./...` passed (app: 66.788 seconds).
- `go vet ./...` passed.
- Windows cross-build and Windows test-binary compilation passed. Windows
  execution was not tested by these compilation checks.
- The new clipboard test asserts that Cancel requests setup cancellation
  without remembering a refusal.
- The original race report and the passing test output are retained in
  `onboarding-20260928/`.
- Install page inspected at 1280x800 and 390x844, including Ubuntu details,
  light and dark appearance. No horizontal overflow was observed.

Source directories `app/`, `linux-ui/` and `runtime-build/linux/site/`
byte-match `/workspace/try-omarchy-linux/fc-onboarding-src` on devbox by
checksum-based rsync dry runs. The tested candidate source patch, including help text at build time, is
`onboarding-20260928/source-final.patch`, SHA256
`5e73226e513632baa16fcd55b8945bed79eb55039f5b9fb0a92f92bc5627b4cb`.
The earlier candidate 3 patch is retained separately as `source.patch`.

## Installed acceptance

Candidate 3 was installed through Ubuntu Software, including its runtime,
from a fresh prepared Ubuntu snapshot. Installed commit:
`d5e102cd8be03be725c7f114f77cf119b6da10fddf332df675077a8d0e7cd01d`.

Observed through the installed application:

- The app-menu launch shows every home action and expandable storage details.
- Try it now starts downloading before any clipboard permission request.
- Cancelling the download exits cleanly and retains the partial download.
  Relaunch resumes at 71 percent, rather than downloading again.
- The clipboard explanation appears after disk preparation, with all choices
  visible. Cancel exits without booting or remembering a refusal. Relaunch
  asks again. Not now boots the trial account and remembers sharing is off.
- The trial desktop starts and its application menu opens. Ctrl+Alt+G releases
  input and the normal shutdown action stops the guest.
- Customize creates a personal account through the actual Omarchy setup.
  A terminal reports `ana`. This account test reuses the verified guest cache;
  it is not a second full-network installation test.
- Settings can enable sharing again. The GNOME permission dialog has clipboard
  enabled and remote interaction disabled. Refusing permission still boots.
- Uninstalling in Software with Keep, then reinstalling, retains the personal
  account and `~/onboarding-check.txt`, whose contents remain `onboarding-kept`.
- A 256 MB temporary filesystem exercises insufficient storage. Setup is
  blocked with the available space and Choose another folder action visible.
  The temporary mount was removed afterward.

Software's Open action failed with an AppArmor user-namespace denial in this
VM; launching from the application menu worked. No security policy was changed.
Help now documents the working route. This observation does not establish the
behavior of every Ubuntu installation.

Candidate 3 also revealed an unnecessary error window after deliberately
refusing GNOME clipboard permission. The final source suppresses that window
for a refusal while retaining error reporting for unexpected portal failures.
The final signed candidate 4 passed the focused regression check. Candidate 3
updated through its configured signed repository using `flatpak update`.
Installed commit exactly matches:
`45218ae8e5ddd8342c3ddcd9d0622e42152cbe6654ccf706edde89a4281eb027`.
The release assets and SHA256SUMS are preserved on devbox in
`/workspace/try-omarchy-linux/fc-release4`; all three hashes passed validation.
The matching site is `/workspace/try-omarchy-linux/fc-site4`.

For the permission regression fixture, the test user's saved refusal marker
was removed so the same GNOME dialog would appear again. Refusing it now starts
the guest without an error popup and saves sharing as off. The desktop boots;
`whoami` and `cat ~/onboarding-check.txt` still print `ana` and
`onboarding-kept`. Normal shutdown succeeds. This final check covers the update
and the last refusal-popup change; the entire fresh-install journey was run on
candidate 3, not repeated on candidate 4.

Selected evidence: [fresh home](onboarding-20260928/home.png),
[resuming download](onboarding-20260928/resuming.png),
[consent](onboarding-20260928/final-consent.png),
[low storage](onboarding-20260928/low-space.png), and
[saved account after update](onboarding-20260928/final-retained.png).

The test machine is a fresh clone of the prepared Ubuntu 24.04 snapshot,
`/var/tmp/try-omarchy-onboarding-20260928/desktop-overlay.qcow2` on devbox.
SSH is forwarded on 22261 and VNC on 5922. The previous candidate 2 VM is
powered off and preserved. GUI input and screenshots use the `cleanview`
omabox viewer. The HTTPS site in this VM is a private stand-in.
At handoff, both guest and test VM are shut down, the disk is preserved,
and the isolated viewer and its SSH tunnel are stopped.

The isolated viewer needed its own minimal Hyprland configuration because
its copied config referenced an unavailable bootstrap file. Only the box's
configuration changed; the real desktop was not used or edited.

## Remaining acceptance

- Publish and smoke-test the GitHub README/release download path and signed
  package repository when authorized. No separate website is required.
- Exercise migration from the exact published preview 1 package. The signed
  update tested here was candidate 3 to candidate 4.
- Outside testers complete physical-hardware acceptance; this is not an owner
  desktop task. Complete other supported-desktop acceptance. Nested
  Ubuntu GNOME testing does not establish GPU, audio or device compatibility.

## Release boundary

The public Pages URL and Pages API returned 404 during the initial audit.
The public entry point is now the GitHub README and release assets. The signed
repository and updated app need publication before new users can use that path;
landing-page publication is not a release requirement. No commit, push, PR, tag or deployment has been
made during this work.

## Subsequent documentation direction

The owner selected the GitHub README and release downloads as the public entry
point, following the Windows repository. README, help and release instructions
were updated after the candidate build. These documentation changes do not
change the tested app binaries. The earlier landing-page changes remain in the
working tree but are not required for launch. tryomarchy.com work is deferred.
