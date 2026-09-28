# Phase 10Q backup safety and error queue

Observed 2026-09-27. This is a private correction checkpoint after the Phase
10P audit, not a release candidate. No commit, push, PR, upload or physical
test occurred.

## Exact identities

| Item | Identity |
| --- | --- |
| Starting HEAD | `e9d8ecd9d9d6576402becb8fa97c16e46edd89ae` |
| Phase 10Q source | `/home/bts/.cache/try-omarchy-phase10q-source`, 747 files, byte-matched to devbox `/workspace/try-omarchy-linux/phase10q-backup-safety-src` |
| [Phase 10Q delta](phase10q/source-delta.tar.gz) | SHA256 `ff8d6db4bbfae1dba56c82fe788d7d202a1fe08bcdb8466cfd512ed455e102fe`; eight files over the Phase 10P source, [manifest](phase10q/source-delta.json), [reconstruction check](phase10q/reconstruction-check.txt) |
| [Phase 10Q Flatpak](phase10q/phase10q-backup-safety.flatpak) | SHA256 `accfc5823724d973cb690f7fa603e7dd0537537c6a2bdce32807edae07f4e736`; installed OSTree `dcea8acfa2f32fb77b2e51ca20bf77f9643795197fb5141f292a0ec18a8fe0bf` |
| Guest | Unchanged from Phase 10P, compatibility revision 39 |
| Test VM | Disposable child `/workspace/try-omarchy-linux/astra-phase10p-audit/desktop.qcow2` over the preserved Phase 10M overlay, which was not modified |

## Audit findings corrected

The Phase 10P audit found no P0 or P1. It found two P2 defects.

1. Backup could read a disk that another QEMU still held. `flock` never
   conflicts with the OFD byte locks QEMU uses, and the `/proc` scan in the
   portal fallback only sees the backup's own Flatpak PID namespace. A
   crashed launcher does not leave an orphan, because Flatpak kills the
   sandbox with it, and a second app instance is refused. The bypass needed a
   QEMU started outside the app lifecycle.
2. The graphics recovery warning was silently dropped when another runtime
   error window was open, such as a declined GNOME clipboard prompt. It was
   never retried.

## Changes

- Non-portal disks: backup holds a whole-file OFD read lock, so a QEMU starting
  during the backup cannot claim the disk, then rejects any lock held by
  another open file. A QEMU in a separate Flatpak sandbox was detected in the
  test VM, where the old `flock` check passed.
- All disks: backup refuses to publish if the disk size or modification time
  changed while it was read. This covers writers that no lock can see.
- Portal disks skip the OFD check. Inside the sandbox the document portal
  accepts OFD locks locally but fails `F_OFD_GETLK` with EIO, and it does not
  report locks held on the host file. The existing portal fallback and the
  unchanged-disk check apply instead.
- Runtime error windows now queue. A title already open or waiting keeps only
  its newest detail, so repeated failures cannot stack windows.

## Checks on the exact Phase 10Q source

- New tests fail on Phase 10P source and pass on Phase 10Q: a real QEMU holding
  a raw disk is rejected, a late QEMU cannot claim a disk held by backup, a
  disk written mid-backup publishes nothing, and queued errors are shown in
  order without duplicates.
- [Full Linux race suite](phase10q/linux-suite.log) passed with host QEMU 11.1.1
  and Go 1.27.1 inside a throwaway omabox. App tests took 36.3s.
- [Linux vet, Windows vet, Windows cross-build and Windows test compilation](phase10q/cross-checks.log)
  passed. Native Windows tests were not run; the shared backup file changed,
  so Windows CI remains a pending gate.
- The GTK helper was not changed, and its separate SDK test was not rerun.

## Installed GUI results on Phase 10Q

- Clipboard access error open, real Phase 10K virgl failure lines appended to
  the running VM's stderr: the detection was logged while only the clipboard
  window existed. Closing it opened the Graphics error window:
  [before](phase10q/queue-clipboard-first.png), [after](phase10q/queue-graphics-after-close.png).
- Portal restored VM, host process touching the disk every 3 seconds during
  backup: the GUI reported "the guest disk changed while backing up" and the
  destination stayed empty: [refused](phase10q/writer-backup-refused.png).
- Same VM without a writer: backup succeeded,
  [saved](phase10q/clean-backup-saved.png). The archive manifest disk SHA256
  `4f867cec843b4b68ff1f5fb3d42b631a313ba7adf5779376aca511dc3ba44227` matched the
  disk, the kernel and initramfs matched Phase 10P, and it had no shared-folder
  entries.
- Earlier in the same VM on Phase 10P: the guest booted on virgl, the personal
  file `phase10k-keep.txt` kept SHA256 `0a1ce2ae...` after a launcher SIGKILL and
  a clean boot, and the real Phase 10K stderr triggered the detector while
  ordinary ZINK/EGL noise did not.

## Not tested

- A real black guest surface. The nested VM has no GPU; only the recorded
  failure signature was replayed.
- The clipboard collision through an actual declined GNOME prompt. The test
  used the unsupported-desktop clipboard message.
- A GUI backup of the default data folder while another QEMU holds it. The
  kernel lock behavior and the unit test cover it, not the installed GUI.
- A restore from the Phase 10Q archive, and restore cancellation on Phase 10Q.
  The restore code did not change.
- A writer that changes data but keeps size and modification time, and writes
  inside the document portal's attribute cache window at the very end of a
  backup.
