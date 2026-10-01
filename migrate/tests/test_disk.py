from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from omarchy_import.disk import open_disk


class DiskTests(unittest.TestCase):
    def setUp(self):
        scratch = tempfile.TemporaryDirectory()
        self.addCleanup(scratch.cleanup)
        self.disk = Path(scratch.name) / "disk.raw"
        self.disk.write_bytes(b"unchanged trial")

    def child(self, locking):
        code = ("import fcntl, sys; "
                "source = open(sys.argv[1], 'rb'); " + locking +
                "; print('locked', flush=True); sys.stdin.read()")
        child = subprocess.Popen([sys.executable, "-c", code, str(self.disk)],
                                 stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
        self.addCleanup(child.wait)
        self.addCleanup(child.stdout.close)
        self.addCleanup(child.stdin.close)
        self.assertEqual(child.stdout.readline().strip(), "locked")
        return child

    def test_refuses_a_launcher_operation_and_a_qemu_shared_lock(self):
        for locking in ("fcntl.flock(source, fcntl.LOCK_EX)",
                        "fcntl.lockf(source, fcntl.LOCK_SH, 1, 100)"):
            with self.subTest(locking=locking):
                child = self.child(locking)
                with self.assertRaisesRegex(OSError, "in use"):
                    open_disk(self.disk)
                child.stdin.close()
                child.wait()
        self.assertEqual(self.disk.read_bytes(), b"unchanged trial")

    def test_holds_writer_out_until_reader_closes(self):
        code = ("import fcntl, sys; source = open(sys.argv[1], 'r+b'); "
                "fcntl.lockf(source, fcntl.LOCK_EX | fcntl.LOCK_NB, 1, 100)")
        with open_disk(self.disk) as source:
            self.assertEqual(source.read(), b"unchanged trial")
            blocked = subprocess.run([sys.executable, "-c", code, str(self.disk)],
                                     capture_output=True)
            self.assertNotEqual(blocked.returncode, 0)
        allowed = subprocess.run([sys.executable, "-c", code, str(self.disk)], capture_output=True)
        self.assertEqual(allowed.returncode, 0, allowed.stderr)

    def test_refuses_qemu_with_image_locking_disabled(self):
        child = self.child("__import__('ctypes').CDLL(None).prctl(15, b'qemu-system-x86', 0, 0, 0)")
        with self.assertRaisesRegex(OSError, "in use"):
            open_disk(self.disk)
        child.stdin.close()
        child.wait()

    def test_symlink_is_not_followed(self):
        linked = self.disk.parent / "linked.raw"
        linked.symlink_to(self.disk)
        with self.assertRaises(OSError):
            open_disk(linked)
