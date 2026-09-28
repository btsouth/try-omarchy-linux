# Phase 10I private first installation, 2026-09-27

## Result

The exact Phase 10H private Flatpak installed into a new Ubuntu GNOME test
account with no prior Try Omarchy app data. It appeared in GNOME search and
opened from the launcher without flags. The default-location personal setup
downloaded the pinned guest, created a 24 GiB VM disk, completed Omarchy's
account setup and reached a visible, usable Omarchy desktop. A clean guest
poweroff followed by another launcher start returned to the same desktop. The
private server served no guest files during the second start.

This is a clean **account** on an inherited configured Ubuntu 24.04.5 GNOME
VM, with installation by Flatpak CLI from a private bundle. It is not a clean
OS or graphical Software installation test, and the guest source is still a
loopback fixture. Phase 10A remains open for the intended distribution route.

## Exact identities and isolation

- Checkout: `/home/bts/Projects/try-omarchy-linux-core`, `linux-core`, HEAD
  `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae`. No source code changed in
  this slice. The Phase 10H [source delta](phase10h/source-delta.tar.gz), SHA256
  `f64f5af024a6cdfff011bc0b29d67d3a2a37afbc13f609f7d67af1bb2155bb0d`,
  reconstructs the installed candidate from the retained snapshot. All work
  remains local, uncommitted and private.
- [Bundle](phase10h/phase10h-storage-final.flatpak) SHA256
  `1ebd5fab35c0c8e9901f533062b6856faa0bf00c4b50de82ae2d893126a85649`;
  installed OSTree commit
  `0332b6a050e9547fe19c6f71b594b86eb455565ae155e1fb22e7237291ba2514`.
  The [identities](phase10i/installed-identities.txt) were recomputed in the
  new account, and [artifact hashes](phase10i/artifact-sha256.txt) were checked
  locally.
- Private guest compatibility revision 39. `SHA256SUMS` SHA256
  `bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826`;
  installed kernel SHA256
  `8ebc2c71271e000540f0a7545afd8e73504fcb724671f3d38c720b241842b901`;
  installed initramfs SHA256
  `3247b8993430da428b2d0a219bea2786ebff461cfb83255680c654095f37766a`.
  The install-state record pins unpacked rootfs SHA256
  `313839548ae9ceec2905a5d5a9535aa13d7dca97faf4539c90811cfc00e5ce2a`;
  that 7 GiB file was verified by the installer, not independently rehashed in
  this slice. The inherited compressed rootfs SHA256 is
  `afce576526c826c7fb19eec08988469efe2ab41262c3c7ab253315ad76b6e108`.
- Disposable devbox outer VM overlay:
  `/workspace/try-omarchy-linux/phase10i-fresh/desktop-overlay.qcow2`, a new
  child of the stopped Phase 10H overlay. Only this child was resized to 80
  GiB. The final stopped overlay SHA256 is
  `cce554aea610077493d2de26d72219353706a0cb9d322eea75694f5ee20ef25e`.
  The [launch script](phase10i/launch-desktop-vm.sh) records the QEMU, VNC and
  SSH fixture. GUI input and captures used task-owned omabox `phase10i-fresh`
  and a VNC viewer of this disposable VM. Neither the user's desktop nor the
  inherited disks were used for the GUI work.

## Observed workflow

| Step | Result and evidence |
| --- | --- |
| Empty account | New `fresh10i` account had no app data, 47 GiB free, KVM access, and matching bundle and manifest hashes in [pre-install](phase10i/pre-install.txt). The VM's GNOME installation and Flatpak runtime came from an existing configured backing image. |
| Install and find app | `flatpak install --user` from the private bundle produced the pinned OSTree commit in [install record](phase10i/private-bundle-install.txt). GNOME search showed the [app icon](phase10i/app-search-ready.png); the icon opened the [empty home](phase10i/first-home-ready.png) without command-line flags. |
| Cancel and retry | Initial Launch opened the [location choice](phase10i/first-location-ready.png). Cancel closed setup without a VM disk in [state](phase10i/first-cancel-state.txt). Launching the icon again returned to the empty [home](phase10i/relaunch-home.png). |
| Default personal setup | The default location led to the [personal-account choice](phase10i/first-account.png) and [optional shared-folder choice](phase10i/first-share-choice.png). Choosing a personal account and no shared folder began the [guest download](phase10i/first-download.png) and [unpack](phase10i/first-run-post-download.png). GNOME showed the [Remote Desktop portal consent](phase10i/first-run-boot.png). Share became available only after Remote Interaction was enabled alongside clipboard in [the consent screen](phase10i/first-portal-consent-enabled2.png). |
| First usable desktop | The real guest showed its [account setup](phase10i/guest-personal-setup.png). A synthetic username, full name and hostname were entered; optional email was skipped and UTC selected. The [confirmation](phase10i/guest-setup-next4.png) was accepted. The actual [Omarchy desktop](phase10i/guest-setup-progress.png) rendered. No real account or device data was used. |
| Returning launch | QMP `system_powerdown` produced a clean guest exit in the [shell log](phase10i/installed-shell.log). The launcher then found the [saved 24 GiB disk](phase10i/relaunch-home-installed.png). Clicking Launch returned to the [same desktop](phase10i/relaunch-guest.png). The [returning log](phase10i/returning-shell-tail.txt) records guest ready 14 seconds after boot and a second clean poweroff. The [private server journal](phase10i/private-server-journal.txt) has no requests after the first download at 21:01 UTC. |

The first personal setup took long enough for the five-minute desktop timer to
expire while the user was still completing guest questions. The shell log then
recorded `guest desktop appeared after startup timeout`, and the desktop
became visible. This exercises the late-desktop dismissal path; it is not a
timed first-boot performance measurement.

The 24 GiB VM disk remains in the task overlay for recovery. The first run
used about 13 GiB of the outer VM's initially free space, leaving 34 GiB.
After the second guest poweroff, the outer Ubuntu VM was powered off cleanly.
The task omabox and VNC tunnel were stopped. The prior overlays and private
guest artifacts remain untouched.

## Remaining gates and next work

- The eventual public artifact URL, independent checksum pin and user-facing
  Flatpak distribution channel are not chosen. This bundle's default guest
  URL is `http://127.0.0.1:18090`, served only inside the disposable VM.
- Ubuntu's Software app and Flatpak Software plugin were absent in this
  fixture. Installation was by Flatpak CLI, so graphical install, Software
  removal and fresh-OS dependency behavior are unverified.
- Trial mode, custom/removable location first run, resource failures,
  cancellation during download and resumption need exact-candidate GUI tests.
  Phase 10G and 10H cover separate retained custom-location recovery checks.
- An in-app confirmed default-VM deletion flow, app and guest update and
  rollback, full desktop matrix, full Linux suite, Windows CI and authorized
  physical checks remain release gates. This is not a public release candidate.

Next, implement and test a path-confirming GUI delete action for a disposable
app-owned default VM while retaining shared folders, then rehearse private
updates and rollback across two exact versions. Publication remains separate.
