import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

BUILD = Path(__file__).resolve().parents[1] / "build.py"


class BuildTests(unittest.TestCase):
    def test_linux_tag_build_is_reproducible_and_bootstrap_uses_bundled_copy(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary)
            first = output / "first"
            second = output / "second"
            tag = "linux-v0.0.0-test"
            for folder in (first, second):
                subprocess.run([sys.executable, str(BUILD), "--version", tag, "--output", str(folder)],
                               check=True, capture_output=True)
            for name in ("try-omarchy-import.pyz", "try-omarchy-import.sh"):
                self.assertEqual((first / name).read_bytes(), (second / name).read_bytes())
            # No network helper is needed when the adjacent checksum matches.
            result = subprocess.run(["bash", str(first / "try-omarchy-import.sh"), "--version"],
                                    env=dict(os.environ, PATH="/usr/bin:/bin"),
                                    capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.strip(), f"try-omarchy-import {tag}")

    def test_invalid_tag_is_refused_before_writing_artifacts(self):
        with tempfile.TemporaryDirectory() as temporary:
            result = subprocess.run([sys.executable, str(BUILD), "--version", "bad;tag",
                                     "--output", temporary], capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(list(Path(temporary).iterdir()), [])
