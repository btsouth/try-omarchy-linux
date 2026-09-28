# Releasing the Linux app

The Linux app ships as a Flatpak from its own signed repository on GitHub
Pages, `https://btsouth.github.io/try-omarchy-linux/repo/`, until Flathub
carries it. People install it once from the `.flatpakref` link and then get
updates through Software or `flatpak update`. The addresses live in
[repository.env](../runtime-build/linux/repository.env).

App versions and guest images are separate. App releases are tagged
`linux-app-vX.Y.Z` (previews add `-preview.N`) and carry the Flatpak. Guest
images are tagged `linux-vX.Y.Z`, and each app version pins exactly one of them
in `app/linux_release_linux.go`. A new app version with a newer pin downloads
the new guest at the next launch and rolls back to the previous one if it does
not reach the desktop.

## The signing key

Commits in the repository are signed with the key in
[try-omarchy-repo.gpg](../runtime-build/linux/try-omarchy-repo.gpg),
fingerprint `CA972BE59691FBF4F599C61B3DD184C911E6DC20`. Installed copies trust
only that key for updates, so losing it means every user has to reinstall.
The private key stays on the maintainer's machine in
`~/.local/share/try-omarchy-signing` and is never given to CI. Keep an offline
copy.

## Steps

1. Bump `linuxAppVersion` in `app/version_linux.go` and add a `<release>` to
   the metainfo, with `~` before a preview suffix (`0.1.0~preview.3`) so the
   final `0.1.0` sorts after it.
2. Build from a clean checkout of the release commit:
   `runtime-build/linux/build-flatpak.sh OUT`.
3. Sign it into the site. Reuse the previous site directory when you have it,
   so clients get update deltas:

   ```sh
   GNUPGHOME=~/.local/share/try-omarchy-signing/gnupg \
     gpg --export-secret-keys CA972BE59691FBF4F599C61B3DD184C911E6DC20 |
     runtime-build/linux/publish-repo.sh OUT SITE RELEASE
   ```

   `RELEASE` then holds the bundle, the `.flatpakref`, `flatpak-site.tar.gz`
   and `SHA256SUMS`, and `SITE/commit.txt` names the signed commit.
4. Test those exact files: install from the `.flatpakref`, update from the
   previous release, uninstall and reinstall.
5. Publish a GitHub release with the tag and the four files from `RELEASE`.
6. Run the **Publish Flatpak repository** workflow with the tag. It checks
   `flatpak-site.tar.gz` against `SHA256SUMS` and deploys it to Pages. It never
   builds or signs anything. The first time, set Pages to deploy from GitHub
   Actions in the repository settings.
7. Check that `flatpak remote-ls try-omarchy` on an installed machine lists the
   new commit, and that Software offers the update.

## Moving to Flathub later

Flathub builds and signs its own copy under the same app ID. Installs from
this repository will not switch on their own; when Flathub is live, publish a
final update here that says so in its release notes, and document
uninstalling this copy (keeping data) and installing from Flathub. The VM and
settings live under the app ID, so they carry over.
