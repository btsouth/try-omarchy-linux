# Linux presentation and owner-reported launch failure

## Presentation

README now uses a 10-second animated official pixel wordmark hero, reduced-motion
still and a current desktop screenshot. AppStream uses the screenshot first and
a VP9 WebM introduction second. Homepage links to GitHub. The existing package
backend page points to GitHub and no longer promises unverified preview 1 migration.

Validated with Ubuntu 24.04 appstreamcli: `validate --no-net` passed (one pedantic
hint, no errors or warnings). GNOME Software 46 local metainfo preview loaded both
assets from the isolated HTTPS stand-in. The second gallery item showed a play
button and played through to the desktop image. Local metainfo preview has a
generic icon because it lacks the repository's composed icon metadata; packaged
icon installation is unchanged. This verifies the gallery, not a rebuilt release.

README animation was viewed in the collaborative browser. Media sources and
regeneration instructions are in docs/images/README.md. New public media URLs
require publication before a release references them. No changes were published.

## Ubuntu Software Open failure

The owner reported `ldconfig failed, exit status 256` after successful installation
of candidate 4. Logs show `bwrap: loopback: Failed RTM_NEWADDR: Operation not
permitted` and AppArmor denials for setpcap/net_admin under unprivileged_userns.
GNOME Software's process is unconfined; direct flatpak has its own userns profile.
`flatpak run --command=true com.tryomarchy.TryOmarchy` succeeded in the owner VM.
Application-menu launch passed the earlier fresh candidate 4 first-run journey.

This happens before the app runs. Documentation now explicitly directs Ubuntu
users to the application menu. Software's Open button remains a compatibility
exception, not a fixed app bug. No AppArmor weakening or user VM reset was done.
The owner VM is preserved; media work used the separate onboarding VM.
