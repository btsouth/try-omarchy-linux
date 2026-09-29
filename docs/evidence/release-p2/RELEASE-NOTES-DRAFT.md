Second Linux app preview for outside testing. Requires x86_64 Linux, KVM and Flatpak.

This preview installs from a signed Flatpak repository, so later updates arrive through Software or `flatpak update`.

- The home screen says what setup needs and where your VM lives. **Try it now** goes from install to a working Omarchy desktop in one step.
- Setup shows what it is doing, resumes an interrupted download and can try again in place.
- A short explanation comes before GNOME's clipboard permission, and Settings can turn sharing on or off.
- Ctrl+Alt+G gives the keyboard back to your desktop, and a one-time tip says so.
- Settings is easier to scan, and Backup and recovery can clean up after an interrupted backup or restore.

## Install

Download `com.tryomarchy.TryOmarchy.flatpakref` below and open it with Software, or from a terminal:

```sh
flatpak install --user ./com.tryomarchy.TryOmarchy.flatpakref
```

On Ubuntu, install Software and its Flatpak plugin first, as the [README](https://github.com/btsouth/try-omarchy-linux#get-started) explains. Open Try Omarchy from the application menu. On Ubuntu 24.04, Software's Open button can fail with `ldconfig failed, exit status 256`, and the menu launcher avoids it.

## Coming from preview 1

Preview 1 never updates itself, and your menu keeps opening it while it is installed. Shut down Omarchy, close Try Omarchy, then run:

```sh
flatpak uninstall --user com.tryomarchy.TryOmarchy//master
```

Keep its data when asked, then install this release. Your VM, account and files stay.

## Not covered yet

Physical GPU, audio and microphone, suspend and resume, and monitor changes still need testers; the [hardware checklist](https://github.com/btsouth/try-omarchy-linux/blob/master/docs/LINUX-HARDWARE-TESTING.md) covers them. Camera, live audio switching, gestures, bridged networking, USB passthrough, host-app launching and host authentication are outside this preview.

Source: `SOURCE_COMMIT`. Flatpak commit: `OSTREE_COMMIT`. Check downloads against `SHA256SUMS`.
