# Try Omarchy for Linux help

Try Omarchy runs the Omarchy desktop in a virtual machine on your Linux PC. This
page covers installing, first use, updates, storage and removal. The app links
here from its own messages.

## Install

You need a 64-bit x86 PC with virtualization turned on, Flatpak, and about
15 GB free: Omarchy takes about 13 GB once set up, and Software also installs
the GNOME runtime the first time if you do not have it yet.

1. Open [btsouth.github.io/try-omarchy-linux](https://btsouth.github.io/try-omarchy-linux/)
   and choose **Install with Software**. Software opens and installs Try
   Omarchy, adding Flathub for the runtime if it is missing.
2. Open **Try Omarchy** from your app launcher and choose **Try it now**.

From a terminal:

```sh
flatpak install --user https://btsouth.github.io/try-omarchy-linux/com.tryomarchy.TryOmarchy.flatpakref
```

Ubuntu does not ship Flatpak. Install it first with
`sudo apt install flatpak gnome-software-plugin-flatpak`, then log out and back
in. Fedora, Linux Mint, Pop!_OS and most other distros already have it.

**Try it now** downloads Omarchy (about 2 GB), sets it up and starts it,
signed in as `omarchy` with the password `omarchy`. **Customize** lets you pick
the folder first, and whether to set up your own account inside Omarchy.
Memory, processors and the rest are in **Settings**. If the download stops,
**Try again** or the next launch continues where it left off.

## KVM

Try Omarchy needs KVM, the virtualization built into Linux. When it is missing
the home screen says so and offers **Check again**.

- Turn on virtualization in your firmware settings. It is called Intel VT-x,
  Intel Virtualization Technology, AMD-V or SVM depending on the PC.
- Check that `/dev/kvm` exists: `ls -l /dev/kvm`. If it exists but you cannot
  open it, add yourself to its group with `sudo usermod -aG kvm $USER`, then
  log out and back in.
- Inside another virtual machine, the host must allow nested virtualization.
- VirtualBox or VMware running at the same time can hold the processor's
  virtualization. Close them and try again.

## Space

Setup needs about 13 GB. Omarchy sees a 24 GB disk, but only what it actually
uses takes space on your drive. The home screen shows where the VM lives, what
it uses and what is free, and warns when the drive has less than 2 GB left,
since Omarchy can stop working if it fills up.

To make room, free space on that drive, or choose **Customize** and pick
another folder before setup. An existing VM can move later with **Backup and
recovery**, **Move this VM**.

## Keyboard, window and files

- While the Omarchy window is focused it has your keyboard, Super key
  included, so Omarchy's shortcuts work. The title bar says **Press Ctrl+Alt+G
  to release the keyboard**. Ctrl+Alt+G gives your keyboard back to your
  desktop until you click the Omarchy window again. Clicking another window
  works too.
- The first time, GNOME asks whether Try Omarchy may inhibit shortcuts. Allow
  it. If you refused, Super opens GNOME's overview instead of Omarchy's menu.
  Turn **Inhibit Shortcuts** back on in GNOME Settings, Apps, Try Omarchy.
- Ctrl+Alt+F switches fullscreen on and off. Settings can open it fullscreen
  every time.
- Inside Omarchy, Super+Space opens its menu and Super+K lists every key
  binding.
- Text, images and files move both ways through the clipboard. Dropping files
  on the window sends them to Omarchy. Settings can also share one folder,
  which appears in Omarchy as `/mnt/host`.

### Clipboard permission on GNOME

GNOME only lets apps reach the clipboard through its Remote Desktop
permission, so its dialog talks about remote desktop and pointer control. Try
Omarchy explains this first and uses the permission for the clipboard alone;
it never moves your pointer or types for you. In GNOME's dialog, turn on
**Allow Clipboard Access** and **Allow Remote Interaction**, then press
**Share**. If you choose **Not now** or refuse in GNOME's dialog, Omarchy still
works and file drops still work, and Try Omarchy does not ask again. Turn
sharing back on in **Settings**, **Share the clipboard with Omarchy**.

## Updates

Try Omarchy installs from its own Flatpak repository, so Software shows its
updates like any other app's. From a terminal, `flatpak update` does the same.
Updating keeps your VM, settings and folder choices. An update installed while
Omarchy is running takes effect the next time you open it.

A new app version can bring newer Omarchy system files. Try Omarchy downloads
them at the next launch, keeps your files, and goes back to the previous ones
if the new ones do not start. Updates inside Omarchy (Super+Space, **Update**)
are separate and work as on any Omarchy install.

**Preview 1** was a downloaded file with no update source. Uninstall it and
choose **Keep** under App Settings & Data, then install from the link above.
Your VM opens as before.

## Where your VM is stored

By default the VM lives in the app's own storage:
`~/.var/app/com.tryomarchy.TryOmarchy/data/try-omarchy`. If you chose another
folder during setup, or moved the VM, it lives there instead. The home screen
always shows the current location and how much it uses. **Backup and
recovery** also lists the system files, free space, and any copies kept by a
move or reset.

## Uninstall

- **In Software:** choose **Uninstall**. Under App Settings & Data, **Keep**
  leaves your VM in place for a later reinstall. **Delete** also removes the
  VM stored in the app's own storage.
- **From a terminal:** `flatpak uninstall com.tryomarchy.TryOmarchy` keeps your
  data. Adding `--delete-data` removes it, including a VM in the app's own
  storage.
- A VM in a folder you chose is never removed by uninstalling. Delete that
  folder in your file manager when you no longer want it.
- Backups are ordinary `.zip` files wherever you saved them. Try Omarchy never
  deletes them.
- To delete just the VM and keep the app, use **Delete this VM** in the home
  screen's menu. It removes only a VM in the app's own storage, never a folder
  you chose, a shared folder or a backup.

## Reinstall and coming back

Install again the same way. If you kept the app's data, your VM opens as
before. If your VM is in a folder you chose and the app's data was deleted,
choose **Use existing data folder** from the home screen's menu and pick that
folder.

## Backup and recovery

- **Create backup** saves the VM and its settings as a `.zip` in a folder you
  choose. It excludes shared folders. Keep backups private; they contain your
  files.
- **Restore as a copy** makes a separate VM from a backup and leaves the
  current one in place.
- **Move this VM** copies it to another folder and keeps the original until the
  moved copy has started once.
- **Reset this VM** starts over and keeps the old disk until you remove it.
- If a backup or restore is interrupted, for example by a crash or a full
  drive, the unfinished files stay where they were being written. Try Omarchy
  remembers exactly which files those are, the home screen mentions them, and
  **Remove unfinished files** deletes only those. Nothing else in the folder is
  touched.

## Report a problem

In **Backup and recovery**, **Create diagnostics** saves logs and settings,
never your VM's disk. Look through it before sharing, since logs can mention
local details, then attach it to an issue at
[github.com/btsouth/try-omarchy-linux/issues](https://github.com/btsouth/try-omarchy-linux/issues),
with your distro, desktop and whether you use Wayland or X11.
