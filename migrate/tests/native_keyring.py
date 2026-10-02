"""Opt-in GNOME Keyring integration tests. Run only in an isolated desktop.

    omabox run -- python3 -m tests.native_keyring  # from migrate/

These use disposable credentials, private runtime homes and private buses.
They are deliberately excluded from ordinary unittest discovery.
"""

import base64
import copy
import io
import os
from pathlib import Path
import json
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

from omarchy_import import keyring, textmerge
from omarchy_import.apply import Applier
from omarchy_import.plan import Action, Plan
from omarchy_import.safefs import Destination, Fingerprint
from omarchy_import.system import Runner, system_environment
from omarchy_import.ui import Cancelled, UI
from tests import fixtures
from tests.test_import import ImportCase
from tests.test_keyring import PasswordUI


def item(label, application, value):
    schema = "chrome_libsecret_os_crypt_password_v2" if application == "chromium" else "org.freedesktop.Secret.Generic"
    return {"label": label, "attributes": {"application": application, "xdg:schema": schema},
            "value": base64.b64encode(value).decode(), "type": "text/plain"}


def live_worker():
    """Exercise an already-unlocked daemon while the applier replaces its file."""
    import gi
    gi.require_version("Secret", "1")
    from gi.repository import Gio, GLib, Secret
    request = json.load(sys.stdin)
    daemon = subprocess.Popen(["/usr/bin/gnome-keyring-daemon", "--foreground", "--unlock", "--components=secrets"],
                              stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    daemon.stdin.write(request["password"].encode())
    daemon.stdin.close()
    try:
        connection = Gio.bus_get_sync(Gio.BusType.SESSION, None)
        def call(path, interface, method, args):
            return connection.call_sync("org.freedesktop.secrets", path, interface, method,
                                        args, None, Gio.DBusCallFlags.NONE, 5000, None).unpack()
        deadline = time.monotonic() + 5
        while not connection.call_sync("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus",
                "NameHasOwner", GLib.Variant("(s)", ("org.freedesktop.secrets",)), None, 0, 1000, None).unpack()[0]:
            assert time.monotonic() < deadline
            time.sleep(0.02)
        session = call("/org/freedesktop/secrets", "org.freedesktop.Secret.Service", "OpenSession",
                       GLib.Variant("(sv)", ("plain", GLib.Variant("s", ""))))[1]
        def read(application):
            unlocked, _ = call("/org/freedesktop/secrets", "org.freedesktop.Secret.Service", "SearchItems",
                               GLib.Variant("(a{ss})", ({"application": application},)))
            return bytes(call(unlocked[0], "org.freedesktop.Secret.Item", "GetSecret",
                              GLib.Variant("(o)", (session,)))[0][2])
        assert read("chromium") == b"disposable-target-key"
        relative = ".local/share/keyrings/login.keyring"
        path = Path.home() / relative
        # Force the replacement into the same second to reproduce tracker caching.
        now = time.time_ns()
        os.utime(path, ns=(now, now))
        assert read("chromium") == b"disposable-target-key"
        action = Action(relative, "file", "keys", "merge", content=base64.b64decode(request["merged"]),
                        expected=Fingerprint.of(path.stat()), backup=True)
        with Destination(Path.home()) as destination:
            result = Applier(destination).apply(Plan([action], [], ["keys"], "trial", {"chromium"}))
        assert not result.failures()
        assert read("chromium") == b"disposable-trial-key"
        assert read("other-app") in (b"\0\xff\x80keep\0", b"destination-wins")
        service = Secret.Service.get_sync(Secret.ServiceFlags.OPEN_SESSION, None)
        collection = Secret.Collection.for_alias_sync(service, "default", 0, None)
        Secret.Item.create_sync(collection, None, {"application": "after-import"}, "Added after import",
                               Secret.Value.new("later-secret", -1, "text/plain"), 0, None)
        print(json.dumps({"data": base64.b64encode(path.read_bytes()).decode()}))
    finally:
        daemon.terminate()
        daemon.wait(timeout=2)


class NativeKeyringTests(ImportCase):
    path = ".local/share/keyrings/Default_keyring.keyring"

    @classmethod
    def setUpClass(cls):
        cls.runner = Runner()
        cls.trial_item = item("Chromium Safe Storage", "chromium", b"disposable-trial-key")
        cls.old_browser = item("Chromium Safe Storage", "chromium", b"disposable-target-key")
        cls.unrelated = item("Unrelated binary credential", "other-app", b"\0\xff\x80keep\0")
        cls.source_extra = item("Another credential", "other-app", b"source-only-key")
        cls.current_extra = copy.deepcopy(cls.source_extra)
        cls.current_extra["value"] = base64.b64encode(b"destination-wins").decode()
        cls.sources = {}
        cls.targets = {}
        for password in ("", "trial-password"):
            cls.sources[password] = keyring._operate(cls.runner, None if password else fixtures.KEYRING_EMPTY, password,
                                                    [cls.trial_item, cls.source_extra])
        for password in ("", "target-password"):
            cls.targets[password] = keyring._operate(cls.runner, None if password else fixtures.KEYRING_EMPTY, password,
                [cls.old_browser, cls.unrelated, cls.current_extra])

    def seed(self, source_password, target_password):
        fixtures.write(self.trial_home / self.path, self.sources[source_password], fixtures.TRIAL_EDIT, 0o600)
        fixtures.write(self.home / self.path, self.targets[target_password], fixtures.HOME_EDIT, 0o600)
        return self.plan(("browser/chromium",))

    def check_merge(self, source_password, target_password):
        plan = self.seed(source_password, target_password)
        self.assertEqual(self.actions(plan)[self.path].action, "unlock-keyring")
        before = (self.home / self.path).read_bytes()
        passwords = [value for value in (source_password, target_password) if value]
        keyring.prepare(plan, self.destination, PasswordUI(passwords), self.runner)
        result = self.apply(plan)
        self.assertEqual(result.failures(), [])
        imported = (self.home / self.path).read_bytes()
        self.assertEqual(textmerge.keyring_is_encrypted(imported), bool(target_password))
        actual = keyring._operate(self.runner, imported, target_password)
        expected = [self.trial_item, self.unrelated, self.current_extra]
        self.assertEqual(sorted(actual, key=lambda x: x["label"]), sorted(expected, key=lambda x: x["label"]))
        backup = Path(result.backup_directory) / self.path
        self.assertEqual(backup.read_bytes(), before)
        self.assertEqual(backup.stat().st_mode & 0o777, 0o600)
        self.assertEqual((self.home / self.path).stat().st_mode & 0o777, 0o600)
        self.assertEqual((self.trial_home / self.path).read_bytes(), self.sources[source_password])
        again = self.plan(("browser/chromium",))
        with patch.object(keyring, "_operate") as native:
            keyring.prepare(again, self.destination, PasswordUI([]), self.runner)
            self.apply(again)
        native.assert_not_called()
        self.assertEqual((self.home / self.path).read_bytes(), imported)

    def test_different_passwords_browser_key_and_binary_destination_entry(self):
        self.check_merge("trial-password", "target-password")

    def test_plaintext_source_into_protected_destination(self):
        self.check_merge("", "target-password")

    def test_protected_source_into_plaintext_destination(self):
        self.check_merge("trial-password", "")

    def test_new_keyring_keeps_source_password(self):
        plan = self.seed("trial-password", "")
        (self.home / self.path).unlink()
        plan = self.plan(("browser/chromium",))
        keyring.prepare(plan, self.destination, PasswordUI(["trial-password"]), self.runner)
        self.apply(plan)
        data = (self.home / self.path).read_bytes()
        self.assertTrue(textmerge.keyring_is_encrypted(data))
        self.assertEqual(sorted(keyring._operate(self.runner, data, "trial-password"), key=lambda x: x["label"]),
                         sorted([self.trial_item, self.source_extra], key=lambda x: x["label"]))
        with self.assertRaises(keyring.Locked):
            keyring._operate(self.runner, data, "wrong-password")

    def test_wrong_password_then_correct_password(self):
        plan = self.seed("trial-password", "target-password")
        ui = PasswordUI(["wrong-password", "trial-password", "target-password"])
        keyring.prepare(plan, self.destination, ui, self.runner)
        self.assertIn("did not unlock", ui.stream.getvalue())
        self.assertNotIn("wrong-password", ui.stream.getvalue())
        self.assertEqual((self.home / self.path).read_bytes(), self.targets["target-password"])

    def test_cancelling_destination_prompt_writes_nothing(self):
        plan = self.seed("trial-password", "target-password")
        with self.assertRaises(Cancelled):
            keyring.prepare(plan, self.destination, PasswordUI(["trial-password", Cancelled()]), self.runner)
        self.assertEqual((self.home / self.path).read_bytes(), self.targets["target-password"])
        self.assertFalse((self.home / ".local/state/try-omarchy-import").exists())

    def test_unattended_import_writes_nothing(self):
        plan = self.seed("trial-password", "target-password")
        with self.assertRaises(keyring.KeyringError):
            keyring.prepare(plan, self.destination, UI(interactive=False, stream=io.StringIO()), self.runner)
        self.assertEqual((self.home / self.path).read_bytes(), self.targets["target-password"])
        self.assertFalse((self.home / ".local/state/try-omarchy-import").exists())

    def test_native_repeat_merge_preserves_ciphertext(self):
        data = keyring._operate(self.runner, self.targets["target-password"], "target-password",
                               [self.trial_item], {"chromium"})
        again = keyring._operate(self.runner, data, "target-password", [self.trial_item], {"chromium"})
        self.assertEqual(again, data)

    def test_live_daemon_reloads_and_keeps_entries_on_its_next_write(self):
        merged = keyring._operate(self.runner, self.targets["target-password"], "target-password",
                                 [self.trial_item], {"chromium"})
        with tempfile.TemporaryDirectory(dir=keyring._runtime_directory()) as home:
            env = system_environment()
            env.update(HOME=home, XDG_DATA_HOME=home + "/.local/share", XDG_CONFIG_HOME=home + "/config",
                       XDG_CACHE_HOME=home + "/cache", XDG_RUNTIME_DIR=home + "/run")
            for name in ("DBUS_SESSION_BUS_ADDRESS", "DBUS_STARTER_ADDRESS", "DBUS_STARTER_BUS_TYPE",
                         "GNOME_KEYRING_CONTROL", "DISPLAY", "WAYLAND_DISPLAY"):
                env.pop(name, None)
            Path(env["XDG_RUNTIME_DIR"]).mkdir(mode=0o700)
            directory = Path(env["XDG_DATA_HOME"]) / "keyrings"
            directory.mkdir(parents=True, mode=0o700)
            path = directory / "login.keyring"
            path.write_bytes(self.targets["target-password"])
            path.chmod(0o600)
            (directory / "default").write_text("login\n")
            package = str(Path(keyring.__file__).parent.parent)
            tests = str(Path(__file__).resolve().parents[1])
            code = "import sys;sys.path[:0]=sys.argv[1:];from tests.native_keyring import live_worker;live_worker()"
            result = subprocess.run([self.runner.which("dbus-run-session"), "--", sys.executable,
                                     "-I", "-c", code, package, tests], env=env, timeout=20,
                input=json.dumps({"password": "target-password", "merged": base64.b64encode(merged).decode()}),
                text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            data = base64.b64decode(json.loads(result.stdout)["data"])
        actual = keyring._operate(self.runner, data, "target-password")
        expected = [self.trial_item, self.unrelated, self.current_extra,
                    item("Added after import", "after-import", b"later-secret")]
        self.assertEqual(sorted(actual, key=lambda x: x["label"]), sorted(expected, key=lambda x: x["label"]))

    def tearDown(self):
        runtime = Path(f"/run/user/{os.getuid()}")
        self.assertEqual(list(runtime.glob("try-omarchy-keyring-*")), [])


if __name__ == "__main__":
    unittest.main()
