# Bring your trial into an installed Omarchy

You can keep the settings, themes, keybindings, apps, files and projects you
made in Try Omarchy when you install Omarchy on your computer.

## Export before installing

Inside the trial, open a terminal and run:

```sh
try-omarchy-export
```

Choose what to bring over. The export includes settings, your folders, app
data and the apps you added. Browser profiles, keys and sign-ins are off by
default; select them only if you want to carry them over too.

The command writes `omarchy-export-<date>.tar.gz` to your shared host folder
when one is available, otherwise to your home inside the trial. You can choose
another destination with `try-omarchy-export /path/to/folder`.

Copy the archive out of the trial before replacing your Linux installation.
Use the shared folder, Files, or a USB drive. Check that the copy exists on
the host or USB drive. Shared host folders themselves are not exported.

## Install Omarchy

Download the ISO from [omarchy.org](https://omarchy.org/), verify its checksum,
and follow the [official installation guide](https://omarchy.org/manual/getting-started/).
Back up your existing Linux files too. A full-disk installation erases the
selected drive, including any Try Omarchy data stored there. Keep your export
on a separate drive if you will erase this one.

To keep your current Linux installation, use the installer's free-space option
and read the [dual-boot guide](https://omarchy.org/manual/dual-boot-install/).
Prepare unallocated space first and confirm the target drive before installing.

On the new Omarchy desktop, run **Update > Omarchy** (or `omarchy update`)
before importing apps.
Fresh ISO installations may not have their online package indexes yet. The
importer asks you to update first when those indexes are missing.

## Import the archive

After installing Omarchy, extract the archive and run the `import.sh` inside
it as your normal account. It shows what it can bring over, lets you choose,
and asks before changing anything. Add `--dry-run` to inspect the plan first.
The archive contains its own importer, so it needs no download. When the
import finishes, log out and back in.

Older Try Omarchy images produce an archive with `restore.sh` instead. Use
that script for those archives, or update the guest before exporting to use
the newer importer described here.

## Read a trial disk directly

If you still have the Linux trial data folder, you can run the importer on
the installed Omarchy without exporting first:

```sh
python3 try-omarchy-import.pyz --data /path/to/try-omarchy
```

Use the folder that contains `vm/disk.raw`. The home screen's **Storage**
page shows where your VM lives. Shut down the trial before importing and keep
it closed until the import finishes. The
importer reads the disk without changing it; a running VM or another disk
operation is refused.

If the trial was interrupted, the importer recovers its filesystem journal in
a temporary snapshot. The original disk stays unchanged. The trial account
can have a different name or user ID; copied files belong to your installed
account. Use `--user <trial-account>` to choose among multiple trial accounts.

Without `--data`, the importer looks in this account's native and Flatpak
Try Omarchy folders and follows saved locations. A folder on an old Linux
installation, another account or another drive should be selected explicitly
with `--data`. It also supports trials on Windows drives.

For contributors, build the standalone importer from the repository with
`python3 migrate/build.py --version dev --output /tmp/importer`. The output
is `/tmp/importer/try-omarchy-import.pyz`.

## What happens to your files

Only changes you made in the trial come over. Untouched defaults keep the
new install's version. When defaults changed between Omarchy versions, the
importer combines your changes with the newer defaults where it can.

If a setting changed on both sides, choose which version stays in place.
Replaced files are backed up under
`~/.local/share/try-omarchy-import/backups`. Documents with conflicting names
are saved alongside the existing copy, with `(from Try Omarchy)` in the name.

The virtual display setup, shared-folder links, VM integration, caches and
hardware-specific monitor settings stay behind. Added packages and Flatpak
apps can be reinstalled. Services and group memberships are listed for you
to review; they are never enabled automatically.

Close your browser before importing its profile. The importer brings its
keyring entries along so saved passwords and sessions can still be read. It
preserves unrelated entries already on this computer. If either keyring is
protected by a password, those entries cannot be merged automatically; saved
sign-ins may need to be restored separately or entered again. Use your
browser or password manager's own export before installing if you need it,
and keep that export private too.

Run the import again after an interruption. It skips completed work, keeps
later edits and deletions, and can pick up new changes from the trial.

Import only a trial you set up and trust. Settings can run programs, and
exports containing browser profiles, keys or sign-ins contain private data.
Keep those archives private and delete them when you no longer need them.
