"""Keep a trial disk unchanged while the importer reads it."""

import ctypes
import fcntl
import os
from pathlib import Path
import stat


class FileLock(ctypes.Structure):
    _fields_ = [("kind", ctypes.c_short), ("whence", ctypes.c_short),
                ("start", ctypes.c_longlong), ("length", ctypes.c_longlong),
                ("pid", ctypes.c_int)]


def _qemu_has_open_disk(metadata):
    # The document portal disables QEMU's byte locks. Check the caller's
    # QEMU processes too, comparing the underlying file rather than its path.
    for process in Path("/proc").iterdir():
        if not process.name.isdecimal():
            continue
        try:
            if process.stat().st_uid != os.getuid() or \
                    not (process / "comm").read_text().strip().startswith("qemu-system"):
                continue
            for descriptor in (process / "fd").iterdir():
                try:
                    opened = descriptor.stat()
                except FileNotFoundError:
                    continue
                if (opened.st_dev, opened.st_ino) == (metadata.st_dev, metadata.st_ino):
                    return True
        except (FileNotFoundError, ProcessLookupError):
            continue
    return False


def open_disk(path):
    """Return a read-only handle held until unmount, or refuse an active disk.

    flock conflicts with launcher backup/reclaim operations. A shared byte
    lock prevents QEMU opening the disk for writes; querying a write lock
    also detects QEMU's shared image locks. No disk bytes are changed.
    """
    descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
    source = os.fdopen(descriptor, "rb")
    try:
        metadata = os.fstat(descriptor)
        if not stat.S_ISREG(metadata.st_mode):
            raise OSError("the trial disk is not a regular file")
        try:
            fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
            fcntl.lockf(descriptor, fcntl.LOCK_SH | fcntl.LOCK_NB)
        except BlockingIOError:
            raise OSError("the trial disk is in use; shut Omarchy down and try again") from None
        requested = FileLock(fcntl.F_WRLCK, os.SEEK_SET, 0, 0, 0)
        response = fcntl.fcntl(descriptor, fcntl.F_GETLK, bytes(requested))
        if FileLock.from_buffer_copy(response).kind != fcntl.F_UNLCK or \
                _qemu_has_open_disk(metadata):
            raise OSError("the trial disk is in use; shut Omarchy down and try again")
        return source
    except BaseException:
        source.close()
        raise
