"""Password-dependent planning and preflight must never write destination files."""
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from omarchy_import import keyring, report
from omarchy_import.apply import Applier, load_journal
from omarchy_import.plan import Action, Plan
from omarchy_import.safefs import Destination, Fingerprint
from omarchy_import.ui import Cancelled, UI
from tests.test_import import ImportCase


class PasswordUI(UI):
    def __init__(self, passwords):
        super().__init__(interactive=False, stream=io.StringIO())
        self.interactive = True
        self.passwords = iter(passwords)

    def password(self, prompt):
        answer = next(self.passwords)
        if isinstance(answer, Exception):
            raise answer
        return answer


class KeyringTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.home = self.root / "home"
        self.home.mkdir()
        self.source = self.root / "source.keyring"
        self.source.write_bytes(b"protected-source")
        self.path = ".local/share/keyrings/login.keyring"
        target = self.home / self.path
        target.parent.mkdir(parents=True)
        target.write_bytes(b"protected-target")
        self.destination = Destination(self.home)
        self.addCleanup(self.destination.close)
        self.action = Action(self.path, "file", "keys", "unlock-keyring",
                             content=b"protected-source", source=self.source,
                             expected=Fingerprint.of(target.stat()))
        self.plan = Plan([self.action], [], ["keys"], "trial", {"chromium"})

    def test_cancel_keeps_destination_unchanged(self):
        with self.assertRaises(Cancelled), patch.object(keyring, "_operate") as native:
            keyring.prepare(self.plan, self.destination, PasswordUI([Cancelled()]), None)
        native.assert_not_called()
        self.assertEqual((self.home / self.path).read_bytes(), b"protected-target")
        self.assertFalse((self.home / ".local/state/try-omarchy-import").exists())

    def test_unattended_import_refuses_protected_keys_before_writes(self):
        with self.assertRaisesRegex(keyring.KeyringError, "interactively"), patch.object(keyring, "_operate") as native:
            keyring.prepare(self.plan, self.destination, UI(interactive=False), None)
        native.assert_not_called()
        self.assertEqual((self.home / self.path).read_bytes(), b"protected-target")

    def test_bad_passwords_are_bounded_and_not_logged(self):
        ui = PasswordUI(["first-secret", "second-secret", "third-secret"])
        with self.assertRaisesRegex(keyring.KeyringError, "still locked"), \
                patch.object(keyring, "_operate", side_effect=keyring.Locked()) as native:
            keyring.prepare(self.plan, self.destination, ui, None)
        self.assertEqual(native.call_count, 3)
        self.assertNotIn("secret", ui.stream.getvalue())
        self.assertEqual((self.home / self.path).read_bytes(), b"protected-target")

    def test_changed_destination_is_refused_before_password(self):
        (self.home / self.path).write_bytes(b"later edit")
        with self.assertRaisesRegex(keyring.KeyringError, "changed while"), patch.object(keyring, "_operate") as native:
            keyring.prepare(self.plan, self.destination, PasswordUI([]), None)
        native.assert_not_called()

    def test_changed_source_is_refused_before_password(self):
        self.source.write_bytes(b"later source")
        with self.assertRaisesRegex(keyring.KeyringError, "changed while"), patch.object(keyring, "_operate") as native:
            keyring.prepare(self.plan, self.destination, PasswordUI([]), None)
        native.assert_not_called()

    def test_prepared_merge_keeps_password_protection_and_requests_backup(self):
        with patch.object(keyring, "_operate", side_effect=[[], [], b"protected-merged"]):
            keyring.prepare(self.plan, self.destination, PasswordUI(["source", "target"]), None)
        self.assertEqual(self.action.action, "merge")
        self.assertTrue(self.action.backup)
        self.assertEqual(self.action.content, b"protected-merged")
        self.assertEqual((self.home / self.path).read_bytes(), b"protected-target")

    def test_lost_password_protection_is_refused(self):
        with self.assertRaisesRegex(keyring.KeyringError, "protection"), \
                patch.object(keyring, "_operate", side_effect=[[], [], b"[keyring]\n"]):
            keyring.prepare(self.plan, self.destination, PasswordUI(["source", "target"]), None)

    def test_unprepared_plan_cannot_be_applied(self):
        with self.assertRaisesRegex(ValueError, "unlock"):
            Applier(self.destination).apply(self.plan)
        self.assertFalse((self.home / ".local/state/try-omarchy-import").exists())


class KeyringPlanTests(ImportCase):
    path = ".local/share/keyrings/Default_keyring.keyring"

    def test_protected_source_defers_unlock_and_dry_run_reports_password(self):
        (self.trial_home / self.path).write_bytes(b"protected-source")
        with patch.object(keyring, "_operate") as native:
            plan = self.plan(("browser/chromium",))
            action = self.actions(plan)[self.path]
            self.assertEqual(action.action, "unlock-keyring")
            self.assertEqual(plan.keyring_apps, {"chromium"})
            self.assertIn("password", "\n".join(report.plan_summary(plan)))
            self.assertIn("unlock and merge", report.plan_details(plan))
        native.assert_not_called()

    def test_protected_destination_defers_unlock(self):
        (self.home / self.path).write_bytes(b"protected-target")
        self.assertEqual(self.actions(self.plan(("keys",)))[self.path].action, "unlock-keyring")

    def test_symlink_destination_is_skipped_without_unlocking(self):
        target = self.home / self.path
        target.unlink()
        target.symlink_to(self.trial_home / self.path)
        with patch.object(keyring, "_operate") as native:
            self.assertEqual(self.actions(self.plan(("keys",)))[self.path].action, "skip")
        native.assert_not_called()

    def test_keyring_write_failure_leaves_browser_untouched_and_resumable(self):
        profile = self.home / ".config/chromium/Default/Preferences"
        before = profile.read_bytes()
        plan = self.plan(("browser/chromium",))
        # Simulate a keyring edit after preflight, before its replacement.
        (self.home / self.path).write_bytes(b"changed-during-import")
        result = self.apply(plan)
        self.assertEqual(profile.read_bytes(), before)
        self.assertTrue(any("keyring could not" in item.detail for item in result.results))
        self.assertNotIn(".config/chromium", load_journal(self.home))


if __name__ == "__main__":
    unittest.main()
