# Flathub submission

Flathub is the intended home for Try Omarchy for Linux. Until it accepts the
app, releases come from the app's own repository (see
[releasing](LINUX-RELEASING.md)). This page lists what is ready and what needs
someone outside this repository.

## Ready

- The manifest builds offline in the pinned Flathub build image and passes
  `flatpak-builder-lint manifest` with no exceptions.
- The metainfo passes `flatpak-builder-lint appstream` once
  `docs/LINUX-HELP.md` is on `master`; it has release notes, a help link,
  keywords, branding colors and an OARS rating.
- `runtime-build/linux/make-flathub-submission.py TAG COMMIT DIR` writes the
  submission: the manifest built from the tagged commit, the QEMU patches, the
  offline Go sources, the clipboard helper source and `flathub.json`
  (`x86_64` only).

## Needs a decision or approval

- **Name and icon.** The app uses the Omarchy name and mark. Flathub reviewers
  ask for permission from the trademark holder when an app is not published by
  the upstream project. Get written approval from the Omarchy maintainers, or
  publish under their organization.
- **App ID verification.** `com.tryomarchy.TryOmarchy` needs control of
  `tryomarchy.com`. After acceptance, Flathub's developer portal gives a token
  for `https://tryomarchy.com/.well-known/org.flathub.VerifiedApps.txt`.
- **Permissions review.** Expect questions about `--device=kvm` (the VM),
  `--share=network` (guest download and guest networking),
  `--filesystem=xdg-run/pipewire-0` (audio and microphone) and
  `--talk-name=org.kde.StatusNotifierWatcher` (tray icon on KDE). Clipboard,
  shared folders and backups already go through portals.
- **Screenshots.** Flathub wants several captioned screenshots at stable URLs.
  Replace the single `master` image with tagged images of the home screen, the
  Omarchy desktop and Settings.
- **Existing installs.** People who installed from this repository keep
  updating from it. When Flathub goes live, publish one last update here whose
  notes explain the switch, and document uninstalling (keeping data) and
  installing from Flathub.

## Submitting

1. Tag the release commit and push the tag.
2. Run the generator with that tag and commit, and open the new-app pull
   request on `flathub/flathub` with its output, following Flathub's
   submission guide.
3. Answer review questions from the list above.
