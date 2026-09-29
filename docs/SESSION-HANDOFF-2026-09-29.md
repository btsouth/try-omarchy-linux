# Try Omarchy Linux handoff, 2026-09-29

Supersedes `SESSION-HANDOFF-2026-09-28.md` for release status.

## State

Branch `linux-first-class` in `/home/bts/Projects/try-omarchy-linux-core`.
Everything is committed locally. Nothing was pushed, tagged, released or
deployed. The release notes draft and evidence under `docs/evidence/release-p2/`
are uncommitted (PNG files are ignored by the repo rule).

Release candidate 6 is built, signed and verified:

- Source built: `99d4492`. Final branch head `46164f7` only adds docs and the
  install page, which do not go into the bundle. The signed site's
  `index.html` comes from `46164f7`.
- Flatpak commit `894a951be58d1bfeb482d62ec01d554289e8982ec862143f742d69ce1b3066bc`.
- Files on devbox in `/var/tmp/try-omarchy-rel-p2/release/`: the bundle,
  `.flatpakref`, `flatpak-site.tar.gz` and `SHA256SUMS` (hashes in
  `docs/evidence/release-p2/checks.txt`). Signed site in `.../site/`.
- The repository was signed fresh, with no delta history. Preview 1 had no
  repository, so nothing needs deltas from it.

## Passed on candidate 6

- Full Linux race suite, vet, Windows vet, Windows cross-build and test
  compilation, and GTK vet and tests, inside the signed bundle with packaged
  QEMU 11.1.1 and Go 1.27.1.
- Clean Ubuntu 24.04 account: `.flatpakref` opened in Software, installed with
  the real icon and "Try Omarchy" source, and opened from the app menu. Try it
  now downloaded the real `linux-v0.1.0` guest from GitHub, the clipboard
  explanation came after disk preparation, the GNOME grant worked, and the
  desktop was reached. `whoami` printed `omarchy`, and text copied in Ubuntu
  pasted inside Omarchy.
- Preview 1 migration: the exact published preview 1 bundle (SHA verified)
  created a VM with a marker file. After preview 2 was installed and preview 1
  removed with `flatpak uninstall --user com.tryomarchy.TryOmarchy//master`,
  preview 2 found the VM as ready and booted it, and the marker survived.
- Signed update from candidate 5 to candidate 6 through Software's Updates page
  worked with no password. Settings, home and a clean shutdown through the
  close dialog passed afterwards, in light and dark.

## Fixed this session

- Settings drew a grey block instead of its buttons after coming from the home
  screen. Hiding or showing the footer divider during layout left the buttons
  undrawn. The divider now fades instead (`5670fd9`). This was reproduced and
  confirmed fixed in the installed app.
- A home test compared a `~`-shortened path with the full path, so it failed
  when TMPDIR was under HOME (`99d4492`).
- Preview 1 migration steps were added to the README, help and install page
  (`46164f7`). Preview 1 stays installed alongside and keeps winning the menu
  until it is removed.

## Known, not fixed

- Software's page before install shows a placeholder icon and "No Screenshots"
  for a `.flatpakref`. After install the icon is right. Screenshots will stay
  missing until `docs/images/*` are on `master`, because the metainfo points at
  raw GitHub `master` URLs (404 today). Pushing fixes the screenshots. The
  pre-install placeholder is a GNOME Software 46 limitation.
- Software's Open button can still fail on Ubuntu 24.04 with the AppArmor
  `ldconfig` error. The documented workaround is the app menu.
- Physical hardware acceptance is still with outside testers.
- The Enter-opens-terminals report was the review viewer (TigerVNC under the
  host's Omarchy dropping a Super release), not the app.

## To publish (needs Brandon's go-ahead; nothing here has been done)

Use `/home/bts/.local/bin/gh` and check that `gh api user --jq .login` is `btsouth`.

1. Push `linux-first-class` to the `linux` remote and open a PR into
   `btsouth/try-omarchy-linux` master. Wait for CI and address any comments,
   then merge. This makes the README images and metainfo screenshot URLs live.
2. Create the prerelease `linux-app-v0.1.0-preview.2` on the merge commit.
   Attach the four files from devbox `/var/tmp/try-omarchy-rel-p2/release/`.
   Use `docs/evidence/release-p2/RELEASE-NOTES-DRAFT.md` as the body and fill in
   SOURCE_COMMIT and OSTREE_COMMIT. If the merge commit differs from
   `46164f7`, the bundle is still valid, since only docs changed; say which
   commit it was built from.
3. In repo Settings, set Pages to deploy from GitHub Actions (first time only).
   Then run the **Publish Flatpak repository** workflow with the tag.
4. Smoke test from a clean account: download the `.flatpakref` from the
   release, install, run the first launch, and check `flatpak remote-ls
   try-omarchy`. Check Software shows the screenshots.
5. Record results in the release gates doc.

## Environment notes

- devbox `/workspace` is full (about 800 MB free), and Docker keeps its data
  there. Most of the space is other projects. Run heavy work under
  `/var/tmp` on the root disk. `/var/tmp/try-omarchy-rel-p2/check.sh` shows how
  to run the suite with all scratch space there.
- The agent test VM (`/var/tmp/try-omarchy-onboarding-20260928`, SSH 22261) is
  powered off. It holds candidate 6 with a fresh trial VM. Its HTTPS stand-in
  serves candidate 6 at the btsouth.github.io address; candidate 4's site is
  kept in `/srv/pages/try-omarchy-linux.cand4`.
- The owner review VM (22262/5923) was not touched this session, other than
  reading logs earlier.
