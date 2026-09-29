# Linux hardware testing

Use the [0.1.0 preview 4 bundle](https://github.com/btsouth/try-omarchy-linux/releases/tag/linux-app-v0.1.0-preview.4),
and include its SHA256 with the report. Physical results are still
pending; desktop VM tests do not establish hardware support.

Use a disposable VM inside Try Omarchy. Keep an independent backup before
trying recovery operations on personal files.

Record distro/version, desktop and Wayland/X11 session, CPU, GPU/driver,
kernel, Flatpak runtime and whether `/dev/kvm` is accessible. Do not include
private files or unreviewed diagnostic bundles.

1. Install, find Try Omarchy in the app launcher, and choose Set up Omarchy.
   Check that the guest desktop draws fully and keyboard capture can be released
   with Ctrl+Alt+G. Close and reopen three times; also reboot from inside Omarchy.
2. Copy Unicode text, a PNG and a file in both directions. On GNOME, test portal
   consent, denial and a later retry. Drop a file from the host file manager and
   confirm its contents inside Omarchy.
3. Play audio, disable the microphone and restart the VM, then enable it and
   restart again. Confirm the disabled guest cannot record. Try unplugging and
   reconnecting the selected audio device. Audio route changes require a full
   VM stop/start, not a guest reboot.
4. Try fullscreen, resize, fractional scale and moving the window between
   monitors. Check for black surfaces, clipped controls and stuck input.
5. On a laptop, check battery reporting and suspend/resume while the guest is
   idle and while a disposable file transfer is running. Confirm files remain
   intact and input/audio recover.
6. Back up a disposable guest file, restore as a separate copy and open it.
   Move that VM to another folder, restart the app and verify the file. Remove
   the retained original only after confirming the moved copy works.
7. Uninstall with app data kept, reinstall and check that the same VM opens.
   For external storage, disconnect it only after stopping the VM, check the
   unavailable-location message, then reconnect and reattach it.

Report each step as passed, failed or not tested, with the exact trigger for a
failure and a screenshot where useful. Never report camera, gesture, USB,
bridged networking, host-app launching or host authentication as supported:
those are outside the first Linux release.

## Live audio in development builds

This is not in published preview 4. Record the candidate bundle hash when
checking it. With a USB headset and the built-in speakers and microphone:

1. Start playback and recording inside Omarchy. Open Settings while it runs,
   switch each direction independently, and save. Confirm sound and recording
   move to the selected devices without restarting the VM.
2. Switch back and select System default. Change your desktop's default device
   and confirm Omarchy follows it without changing other apps' routes.
3. Unplug and reconnect the selected headset. Check fallback and recovery;
   **Refresh audio devices** updates the list without saving other edits.
4. Shut down and launch again. Confirm the device choices are kept. Start with
   microphone access disabled and confirm selecting an input cannot enable it.

Virtual PipeWire nodes can check routing and disconnection behavior, but do not
establish physical playback or recording acceptance.

## Reports so far

| Date | Host | Hardware | Install | Result |
| --- | --- | --- | --- | --- |
| 2026-09-29 | Debian testing (forky), COSMIC on Wayland (cosmic-comp) | Intel i3-8130U laptop, UHD 620, 16 GB RAM | Preview 3, `--user` Flatpak through COSMIC Store, separate unprivileged account | Worked for a few minutes of use. Keyboard shortcuts and window resizing were smooth. The app gave the VM 2 CPUs and about 6 GB of memory. Audio, clipboard, suspend and the other steps above were not reported. |
