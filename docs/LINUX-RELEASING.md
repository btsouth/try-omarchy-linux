# Releasing the Linux app

People find the app at [tryomarchy.com/linux](https://tryomarchy.com/linux/)
and in the [README](../README.md), and install it from
`https://tryomarchy.com/linux.flatpakref`. That file adds the signed update
repository at `https://flatpak.tryomarchy.com/repo/`, so updates arrive through
Software or `flatpak update`. The addresses live in
[repository.env](../runtime-build/linux/repository.env).

`flatpak.tryomarchy.com` is the Cloudflare Pages project `try-omarchy-flatpak`
in the account that owns tryomarchy.com, with a proxied CNAME to
`try-omarchy-flatpak.pages.dev`. Pages has no bandwidth cap for static files,
which matters because every install downloads the app from it. Installed
copies update from that address, so it must keep working wherever the source
repository lives. The repository summary carries `--redirect-url`, which moves
any install still using an older address the next time it updates.

Preview 2 installed from `https://btsouth.github.io/try-omarchy-linux/repo/`.
Keep deploying each release there too, with the **Publish Flatpak repository**
workflow, so those installs receive the summary that moves them. GitHub does
not redirect Pages sites after a repository transfer, so once the repository
moves, installs that never updated have to reinstall.

tryomarchy.com is the `btsouth/tryomarchy-site` repository on Cloudflare
Pages. It serves the Linux page and a copy of the `.flatpakref` at
`/linux.flatpakref`, which must match the one `publish-repo.sh` writes.

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
   `runtime-build/linux/build-flatpak.sh OUT`. The metainfo's screenshot
   URLs must already load from `master`, because `appstreamcli compose` drops
   any it cannot download and Software then shows "No Screenshots". Check
   `zcat OUT/build/files/share/app-info/xmls/*.xml.gz | grep -c '<screenshot'`
   is not 0.
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
6. Run `runtime-build/linux/deploy-repo.sh TAG` with wrangler logged in. It
   checks `flatpak-site.tar.gz` against `SHA256SUMS` and uploads it to
   flatpak.tryomarchy.com. Then run the **Publish Flatpak repository** workflow
   with the tag for the old GitHub Pages address. Neither builds or signs
   anything.
7. If the `.flatpakref` changed, copy it to `linux.flatpakref` in
   tryomarchy-site and push. Then install from
   `https://tryomarchy.com/linux.flatpakref` on a clean account.
8. Check that `flatpak remote-ls try-omarchy` on an installed machine lists the
   new commit, and that Software offers the update.
