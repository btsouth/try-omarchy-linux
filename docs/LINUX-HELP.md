# Try Omarchy for Linux help

Try Omarchy runs the Omarchy desktop in a virtual machine on your Linux PC. This
page covers installing, first use, updates, storage and removal. The app links
here from its own messages.

Want to keep your trial when installing Omarchy for real? See
[Bring your trial into an installed Omarchy](MIGRATION.md).

## Install

You need a 64-bit x86 PC with virtualization turned on, Flatpak, and about
15 GB free: Omarchy takes about 13 GB once set up, and Software also installs
the GNOME runtime the first time if you do not have it yet.

1. [Download the installer](https://tryomarchy.com/linux.flatpakref), open it
   with **Software** or **Discover**, then choose **Install**. It adds the
   GNOME runtime if missing. [tryomarchy.com/linux](https://tryomarchy.com/linux/)
   walks through the same steps.
2. Open **Try Omarchy** from your app launcher and choose **Set up Omarchy**.

From a terminal:

```sh
flatpak install --user https://tryomarchy.com/linux.flatpakref
```

Each [release](https://github.com/btsouth/try-omarchy-linux/releases) also
carries the installer and a standalone `.flatpak` bundle.

Ubuntu's App Center does not install Flatpaks. Install Software and its
Flatpak support, then log out and back in:

```sh
sudo apt update
sudo apt install flatpak gnome-software gnome-software-plugin-flatpak
```

If Software installs the app but its **Open** button fails with
`ldconfig failed, exit status 256`, open **Try Omarchy**
from your application menu. This worked in our Ubuntu 24.04 test where
Software's launch was blocked by AppArmor.

On Fedora, Software may first ask whether to enable third-party repositories.
Choose **Enable**. The app's GNOME runtime comes from Flathub, which Fedora
ships turned off, and enabling it lets the install use Fedora's own Flathub
setup. If you choose **Ignore**, the install still works, but Flatpak adds a
second Flathub source of its own, named `flathub-1`, for the runtime. From a
terminal, run `sudo fedora-third-party enable` before installing.

On Linux Mint, Software Manager shows the app as `com.tryomarchy.TryOmarchy`
with a generic icon and an **Unverified Flatpak** badge until it is installed.
It shows every app from outside Flathub that way. Once installed, the menu
entry has the right name and icon. Mint names the app's update source
`tryomarchy-origin` rather than `try-omarchy`.

NixOS does not enable Flatpak by default. Add
`services.flatpak.enable = true;` to `/etc/nixos/configuration.nix`, run
`sudo nixos-rebuild switch`, then log out and back in so the application menu
picks up Flatpak apps. Flathub does not need to be added first; the installer
adds it for the app's GNOME runtime. `/dev/kvm` is usable by every user on
NixOS, so no group change is needed.

**Set up Omarchy** asks how you want to sign in, then downloads Omarchy
(about 2 GB), sets it up and starts it. **My own username and password** is selected by default. Choose it and press
**Continue** to use Omarchy's normal account setup. **Quick start** skips that
and signs you in as `omarchy` with the password `omarchy`. Sudo does not ask that account for a password, and SSH accepts only
keys for it, since its password is public. It is meant for a first look; run
`passwd` in Omarchy to change the password. **Choose location** asks where to store
Omarchy first, then asks the same question. The choice is made once, at first
setup. To switch later, back up anything you want to keep, choose **Delete this
VM**, and set it up again.
The launcher waits until Omarchy confirms that its desktop is ready. Finish
account setup or sign in inside the Omarchy window while it waits. If this takes
more than five minutes, the launcher keeps waiting and leaves **Stop Omarchy**
available. If the guest is stuck, stop it and try again; diagnostics stay in the
data folder.

Memory, processors and the rest are in **Settings**. If the download stops,
**Try again** or the next launch continues where it left off.

## Moving from preview 1

Preview 1 was a standalone bundle. It never updates itself, and while it is
installed your application menu keeps opening it instead of a newer release.
To move to the current release and keep your VM:

1. Shut down Omarchy and close Try Omarchy.
2. Remove preview 1 without deleting its data:
   `flatpak uninstall --user com.tryomarchy.TryOmarchy//master`.
   Answer no if Flatpak offers to delete the app's data.
3. Install the current release as described above.
4. Open Try Omarchy. Your VM, account and files are still there.

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

To make room, free space on that drive, or choose **Choose location** and pick
another folder before setup. An existing VM can move later with **Backup and
recovery**, **Move this VM**.

### Give space back after deleting files

Deleting files inside Omarchy frees space for Omarchy, but its disk file on
your drive does not shrink by itself. To give that space back, open
**Settings** while Omarchy runs and choose **Prepare free space** under
**Disk space**, or choose **Reclaim disk space** from the Try Omarchy tray
icon. From a terminal, `flatpak run com.tryomarchy.TryOmarchy -reclaim` does
the same.

Omarchy then fills up to 8 GB of its free space with zeros. That space is in
use on your drive while it works, and at least 4 GB always stays free. A
notification says when it is ready. Shut Omarchy down, and Try Omarchy gives
the space back during shutdown, then says how much the disk file shrank. If
you deleted more than 8 GB, run it again after the next launch. Your files
inside Omarchy are not changed.

Reclaim is unavailable when the folder that holds Omarchy is on a drive that
cannot release unused blocks; Settings says so.

## Keyboard, window and files

- While the Omarchy window is focused it has your keyboard, Super key
  included, so Omarchy's shortcuts work. The title bar says **Press Ctrl+Alt+G
  to release the keyboard**. Ctrl+Alt+G gives your keyboard back to your
  desktop until you click the Omarchy window again. Clicking another window
  works too.
- On X11 desktops such as Cinnamon, MATE and Xfce, Omarchy has the keyboard
  only while the pointer is over its window. Move the pointer out and your
  desktop's shortcuts, panel menus and applets work as usual; move it back and
  Omarchy has the keyboard again.
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

## Settings

Settings uses **General**, **Devices** and **Advanced** pages. General includes
display and startup, resources, disk capacity and shared files. Devices contains
microphone/camera access, audio choices and clipboard permission where available.
Advanced contains display scale, keyboard layout, rendering, network and SSH.
Switching pages keeps your edits. The product header, page navigation, Save and
Cancel stay visible while you scroll; narrow windows stack fields and actions.

**Balanced** sizes the VM automatically while leaving room for your Linux
desktop. **Maximum performance** uses more available resources. **Manual** shows
memory and processor controls; switching profiles retains their values. The
resource estimate is checked again at launch, so it can change as other apps
use memory. **Restore resource defaults** returns to Balanced.

VM settings apply on the next launch. If Omarchy is running, shut it down and
launch it again; rebooting inside the guest does not restart the VM. Audio
device choices and local port forwards can apply during a running session, as
described below. The startup choice applies the next time you open Try Omarchy.

**Start Omarchy when I open Try Omarchy** waits 10 seconds before launching
an existing VM. Choose Settings or Close to stop it. First setup and startup
warnings keep the launcher open so you can review the explanation and actions.

If a save fails, Settings names the failed group and any groups already saved.
The full explanation appears above the current page's controls, ready to read.
Your remaining edits stay in the form. Fix the folder's permissions or free
space, then Save again. Cancel closes the form without saving further changes;
it does not undo groups that were already saved.

## Audio devices

Settings lets you choose separate playback and microphone devices. System
default follows the device selected by your Linux desktop. Microphone access
changes require shutting down Omarchy and launching it again.

Device choices apply when you save while Omarchy is running.
Use **Refresh devices** after connecting a headset; refreshing keeps your
edits without saving them. If a live switch fails, Settings shows the error and
keeps the saved choices for the next launch. You can retry Save.

## SSH and port forwards

**Allow SSH from this computer** on the Advanced page forwards a port on
127.0.0.1 to Omarchy's SSH server. Other computers cannot connect. SSH changes
apply after shutting down Omarchy and launching it again, because Omarchy starts
its SSH server at boot.

**Other local port forwards** takes one forward per line, such as `tcp:8080:80`
to reach port 80 inside Omarchy at `127.0.0.1:8080`. While Omarchy runs, Save
adds and removes these forwards right away. If a port is already in use on this
computer, Settings says so and saves nothing. If the running VM cannot change a
forward, the new list is still saved for the next launch and you can retry Save.

## Camera

In Settings, turn on camera access and choose a camera, then shut down Omarchy
and launch it again. When an app inside Omarchy opens the camera, your desktop
asks for permission. Camera capture stops when the guest app closes the device.
Camera selection and access changes need another shutdown and launch.

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

## Snapshots

A snapshot saves your VM as it is now: its disk, Omarchy's system files and
Try Omarchy's settings for it. Take one before trying something you may want to
undo. Shut down Omarchy, open **Backup and recovery**, choose **Snapshots...**,
then **Create snapshot** and give it a name.

Snapshots are compressed copies kept in the VM's own folder, under
`checkpoints`. Each one can take as much space as the VM uses, so the
Snapshots page and the storage summary show their sizes. Choose a snapshot to:

- **Restore as a copy**: make a separate VM from it in a folder you choose. The
  current VM stays as it is. Open the copy with **Use existing data folder**.
- **Roll back to this snapshot**: replace the current VM with the snapshot.
  The state you are leaving is kept in the VM's folder, so nothing is lost
  yet. Remove it later with **Remove state kept from roll back...** in Backup
  and recovery. The first launch after a roll back uses the system files saved
  with the snapshot; a newer version, if any, is fetched on the launch after.
- **Delete snapshot**: free its space. The VM and other snapshots stay.

Every snapshot is checked against its checksum before it is restored. If a roll
back is interrupted, for example by a crash or power loss, the next time Try
Omarchy opens it finishes or undoes it before showing the VM. Snapshots live
with the VM: **Delete this VM** removes them too. Use **Create backup** for a
copy kept somewhere else.

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
