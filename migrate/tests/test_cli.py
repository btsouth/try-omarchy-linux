import contextlib
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from omarchy_import import cli, selection
from omarchy_import.plan import Group, Inventory
from tests import fixtures


class CliCase(unittest.TestCase):
    def setUp(self):
        scratch = tempfile.TemporaryDirectory()
        self.addCleanup(scratch.cleanup)
        self.base = Path(scratch.name)
        self.root, self.trial_home = fixtures.make_trial(self.base)
        self.home, self.skel = fixtures.make_home(self.base)
        patcher = mock.patch.dict(os.environ, {"HOME": str(self.home),
                                               "TRY_OMARCHY_IMPORT_SKEL": str(self.skel)})
        patcher.start()
        self.addCleanup(patcher.stop)
        self.runner = fixtures.FakeRunner({
            "pacman -Qq": "base\nomarchy\n",
            "pacman -Sl": "extra cowsay 3.8-1\n",
            "id -Gn": "ada wheel\n",
        }, programs={"pacman", "yay", "omarchy-theme-set", "id", "getent", "systemctl"})

    def main(self, *arguments):
        output = io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
            status = cli.main(["--root", str(self.root), *arguments], runner=self.runner)
        return status, output.getvalue()


class CliTests(CliCase):
    def test_missing_indexes_stop_selected_apps_before_copying_any_files(self):
        self.runner.failing.add("pacman -Sl")
        before = (self.home / ".config/hypr/bindings.lua").read_bytes()
        status, _ = self.main("--yes", "--select", "defaults")
        self.assertEqual(status, 1)
        self.assertEqual((self.home / ".config/hypr/bindings.lua").read_bytes(), before)
        self.assertFalse((self.home / ".local/share/try-omarchy-import").exists())
        self.assertNotIn("pacman -S", self.runner.commands())

    def test_flatpak_installed_with_repo_packages_is_used_in_the_same_import(self):
        from omarchy_import.packages import PackagePlan
        from omarchy_import.ui import UI
        plan = PackagePlan(repo=["flatpak"], flatpaks=[("com.example.App", "user")])
        original = self.runner.run
        def run(argv, **kwargs):
            result = original(argv, **kwargs)
            if argv[:2] == ["pacman", "-S"]:
                self.runner.programs.add("flatpak")
            return result
        self.runner.run = run
        with contextlib.redirect_stdout(io.StringIO()):
            steps = cli._install_packages(UI(interactive=False), self.runner, plan)
        self.assertTrue(all(step.ok for step in steps))
        self.assertTrue(any(command.startswith("flatpak install") for command in self.runner.commands()))

    def test_dry_run_json_lists_groups_and_changes_nothing(self):
        before = sorted(path.relative_to(self.home) for path in self.home.rglob("*"))
        status, output = self.main("--dry-run", "--json", "--select", "defaults")
        self.assertEqual(status, 0)
        document = json.loads(output[output.index("{"):])
        ids = [group["id"] for group in document["available"]]
        self.assertIn("settings", ids)
        self.assertIn("browser/chromium", ids)
        self.assertNotIn("browser/chromium", document["groups"])
        self.assertNotIn("keys", document["groups"])
        self.assertEqual(document["packages"]["repo"], ["cowsay"])
        self.assertEqual(document["theme"], "gruvbox")
        after = sorted(path.relative_to(self.home) for path in self.home.rglob("*"))
        self.assertEqual(before, after)

    def test_import_installs_packages_and_sets_the_theme(self):
        status, output = self.main("--yes", "--select", "settings,packages,theme")
        self.assertEqual(status, 0, output)
        self.assertIn(b"name = Ada", (self.home / ".config/git/config").read_bytes())
        commands = self.runner.commands()
        self.assertIn("pacman -S --needed --noconfirm -- cowsay", commands)
        self.assertIn("yay -S --needed --noconfirm -- figlet", commands)
        self.assertIn("omarchy-theme-set gruvbox", commands)
        self.assertIn("Log out and back in", output)
        # sudo forgets the password after the packages and before the theme
        # hooks the import brought can run.
        forget = commands.index("sudo -k")
        self.assertGreater(forget, commands.index("yay -S --needed --noconfirm -- figlet"))
        self.assertLess(forget, commands.index("omarchy-theme-set gruvbox"))

    def test_mise_runs_only_when_its_config_came_over(self):
        self.runner.programs.add("mise")
        self.main("--yes", "--select", "files")
        self.assertFalse(any(command.startswith("mise install") for command in
                             self.runner.commands()))
        self.main("--yes", "--select", "settings")
        self.assertIn("mise install --yes", self.runner.commands())

    def test_the_trials_background_follows_the_theme_switch(self):
        # The trial's background is one of the theme's own, which only shows
        # up here once omarchy-theme-set has switched to that theme.
        self.runner.programs.add("omarchy-theme-bg-set")
        wanted = self.home / ".local/state/omarchy/current/theme/backgrounds/1-gruvbox.jpg"
        original = cli.packages.set_theme

        def set_theme(runner, theme):
            fixtures.write(wanted, b"jpg", fixtures.HOME_EDIT)
            return original(runner, theme)

        with mock.patch.object(cli.packages, "set_theme", set_theme):
            status, output = self.main("--yes", "--select", "theme")
        self.assertEqual(status, 0, output)
        self.assertIn(f"omarchy-theme-bg-set {wanted}", self.runner.commands())

    def test_theme_is_left_alone_when_it_is_already_current(self):
        fixtures.write(self.home / ".local/state/omarchy/current/theme.name", b"gruvbox\n",
                       fixtures.HOME_EDIT)
        self.main("--yes", "--select", "theme")
        self.assertNotIn("omarchy-theme-set gruvbox", self.runner.commands())

    def test_incomplete_steps_make_the_exit_status_nonzero(self):
        self.runner.failing.add("yay -S")
        status, output = self.main("--yes", "--select", "packages")
        self.assertEqual(status, 1)
        self.assertIn("Not finished", output)

    def test_unknown_group_is_rejected(self):
        status, _ = self.main("--yes", "--select", "settings,nope")
        self.assertEqual(status, 2)

    def test_json_needs_dry_run_or_yes(self):
        status, _ = self.main("--json")
        self.assertEqual(status, 2)
        self.assertFalse((self.home / ".config/mise/config.toml").exists())

    def test_mounts_that_could_not_be_undone_are_reported(self):
        from omarchy_import import attach
        errors = io.StringIO()
        original = attach.Session.close

        def failing_close(session):
            original(session)
            return ["umount /tmp/x: busy"]

        with mock.patch.object(attach.Session, "close", failing_close), \
                contextlib.redirect_stdout(errors):
            cli.main(["--root", str(self.root), "--yes", "--select", "settings"],
                     runner=self.runner)
        self.assertIn("--cleanup", errors.getvalue())

    def test_package_names_that_are_options_never_reach_an_installer(self):
        # The probe from the security review: a desc file whose name is a yay
        # option that runs another program.
        fixtures.write(self.root / "var/lib/pacman/local/controlled-1/desc",
                       b"%NAME%\n--makepkg=/tmp/controlled-demo-helper\n", fixtures.TRIAL_EDIT)
        status, output = self.main("--yes", "--select", "packages")
        self.assertEqual(status, 0, output)
        self.assertFalse(any("--makepkg" in command for command in self.runner.commands()))
        self.assertIn("yay -S --needed --noconfirm -- figlet", self.runner.commands())
        self.assertIn("Left out 1 entry in the trial's package list", output)

    def test_the_question_says_checked_items_come_along(self):
        questions = []

        def choose_many(ui, question, options, selected):
            questions.append(question)
            return list(selected)

        with mock.patch.object(cli.UI, "choose_many", choose_many), \
                mock.patch.object(cli.UI, "choose_one", lambda *arguments, **options: 0), \
                mock.patch.object(cli.UI, "__init__", _interactive_ui):
            self.main()
        self.assertEqual(questions, ["What do you want to bring over?\n"
                                     "Everything checked comes along. Press Enter to continue."])

    def test_refuses_to_run_as_root(self):
        with mock.patch("os.geteuid", return_value=0):
            status, _ = self.main("--yes")
        self.assertEqual(status, 2)


def _interactive_ui(ui, interactive=True, use_gum=None, stream=None):
    import sys
    ui.stream = stream or sys.stdout
    ui.interactive = True
    ui.gum = False
    ui.gum_path = None


class LocateTests(unittest.TestCase):
    def test_linux_trial_is_attached_without_searching_windows_drives(self):
        from tests.test_locate import ext4_image
        with tempfile.TemporaryDirectory() as temporary:
            home = Path(temporary)
            data = home / ".var/app/com.tryomarchy.TryOmarchy/data/try-omarchy"
            ext4_image(data / "vm/disk.raw")
            runner = fixtures.FakeRunner()
            session = mock.Mock()
            session.attach_disk.return_value = home / "mounted-trial"
            with mock.patch.dict(os.environ, {"HOME": str(home), "XDG_DATA_HOME": ""}):
                args = cli.build_parser().parse_args([])
                root, install = cli.find_trial(args, cli.UI(interactive=False, stream=io.StringIO()),
                                               runner, session)
            self.assertEqual(install.data_dir, data)
            self.assertEqual(root, home / "mounted-trial")
            session.attach_disk.assert_called_once_with(data / "vm/disk.raw", False)
            self.assertFalse(any(command.startswith("lsblk") for command in runner.commands()))

    def test_bitlocker_drive_is_explained(self):
        tree = {"blockdevices": [{"path": "/dev/nvme0n1p3", "fstype": "BitLocker",
                                  "label": "Windows", "size": 1, "mountpoints": [None]}]}
        runner = fixtures.FakeRunner({"lsblk": json.dumps(tree)}, programs={"lsblk"})
        errors = io.StringIO()
        with mock.patch.object(cli.locate, "find_linux_installs", return_value=[]), \
                contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(errors):
            status = cli.main(["--yes"], runner=runner)
        self.assertEqual(status, 1)
        self.assertIn("encrypted with BitLocker", errors.getvalue())
        self.assertFalse(any(command.startswith("mount") for command in runner.commands()))


class SelectionTests(unittest.TestCase):
    def inventory(self):
        groups = {}
        for group_id, kind, size in (("settings", "settings", 10), ("files/Big", "files", 900),
                                     ("files/Small", "files", 50), ("apps/Huge", "apps", 10**9),
                                     ("apps/Tiny", "apps", 5), ("browser/chromium", "browser", 5),
                                     ("keys", "keys", 1)):
            groups[group_id] = Group(group_id, kind, group_id.split("/")[-1], bytes=size, files=1)
        return Inventory(groups, [])

    def test_defaults_fit_the_free_space(self):
        free = selection.RESERVE_BYTES + 100
        rows = {row[0]: row[2] for row in selection.option_rows(self.inventory(), None, "tokyo", free)}
        self.assertTrue(rows["settings"])
        self.assertTrue(rows["files/Small"])
        self.assertFalse(rows["files/Big"])
        self.assertFalse(rows["apps/Huge"])
        self.assertTrue(rows["apps/Tiny"])
        self.assertFalse(rows["browser/chromium"])
        self.assertFalse(rows["keys"])
        self.assertTrue(rows["theme"])

    def test_keywords(self):
        rows = selection.option_rows(self.inventory(), None, None, 10**12)
        self.assertEqual(selection.resolve_selection("files,keys", rows),
                         {"files/Big", "files/Small", "keys"})
        self.assertEqual(selection.resolve_selection("all", rows), {row[0] for row in rows})
        self.assertNotIn("keys", selection.resolve_selection("defaults", rows))
        with self.assertRaises(selection.SelectionError):
            selection.resolve_selection("bogus", rows)


if __name__ == "__main__":
    unittest.main()
