"""Merge protected GNOME keyrings using private copies and a private Secret Service.

Passwords and decrypted items travel only through pipes. The scratch home lives
in the login runtime directory, never beside the trial or destination files.
"""

import base64
import contextlib
import json
import os
from pathlib import Path
import resource
import signal
import subprocess
import sys
import tempfile
import time

from . import safefs, textmerge
from .system import system_environment


class KeyringError(Exception):
    pass


class Locked(KeyringError):
    pass


def _runtime_directory():
    path = Path(f"/run/user/{os.getuid()}")
    info = path.lstat()
    if not path.is_dir() or path.is_symlink() or info.st_uid != os.getuid() or info.st_mode & 0o022:
        raise KeyringError("A private login runtime directory is required to unlock keyrings.")
    return path


def _operate(runner, data, password, items=None, trial_wins=()):
    bus = runner.which("dbus-run-session")
    daemon = runner.which("gnome-keyring-daemon")
    if not bus or not daemon:
        raise KeyringError("Install gnome-keyring and dbus to import a protected keyring.")
    try:
        runtime = _runtime_directory()
    except OSError:
        raise KeyringError("Run from a logged-in desktop session to unlock keyrings.") from None
    with tempfile.TemporaryDirectory(prefix="try-omarchy-keyring-", dir=runtime) as scratch:
        env = system_environment()
        # D-Bus activation must inherit this home too, not the destination home.
        env.update(HOME=scratch, XDG_DATA_HOME=scratch + "/data",
                   XDG_CONFIG_HOME=scratch + "/config", XDG_CACHE_HOME=scratch + "/cache",
                   XDG_RUNTIME_DIR=scratch + "/run", TMPDIR=scratch)
        for key in ("DBUS_SESSION_BUS_ADDRESS", "DBUS_STARTER_ADDRESS", "DBUS_STARTER_BUS_TYPE",
                    "GNOME_KEYRING_CONTROL", "DISPLAY", "WAYLAND_DISPLAY"):
            env.pop(key, None)
        os.mkdir(env["XDG_RUNTIME_DIR"], 0o700)
        directory = Path(env["XDG_DATA_HOME"]) / "keyrings"
        directory.mkdir(parents=True, mode=0o700)
        path = directory / "login.keyring"
        if data is not None:
            path.write_bytes(data)
            path.chmod(0o600)
        (directory / "default").write_text("login\n")
        payload = {"password": password, "items": items, "trial_wins": sorted(trial_wins),
                   "daemon": daemon, "existing": data is not None}
        package = str(Path(__file__).parent.parent)
        code = "import sys; sys.path.insert(0,sys.argv[1]); from omarchy_import.keyring import _worker; _worker()"
        process = subprocess.Popen([bus, "--", sys.executable, "-I", "-c", code, package],
                                   stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                   stderr=subprocess.DEVNULL, env=env, start_new_session=True)
        try:
            output, _ = process.communicate(json.dumps(payload).encode(), timeout=30)
            result = json.loads(output)
            if result.get("error") == "locked":
                raise Locked("The keyring password did not unlock it.")
            if result.get("error") == "dependencies":
                raise KeyringError("Install python-gobject and libsecret to import a protected keyring.")
            if process.returncode or "error" in result:
                raise KeyringError("The keyring could not be read or merged. Nothing was imported.")
            return result["items"] if items is None else base64.b64decode(result["data"], validate=True)
        except (ValueError, KeyError, subprocess.TimeoutExpired):
            raise KeyringError("The keyring could not be read or merged. Nothing was imported.") from None
        finally:
            if process.poll() is None:
                with contextlib.suppress(ProcessLookupError):
                    os.killpg(process.pid, signal.SIGTERM)
                try:
                    process.wait(timeout=2)
                except subprocess.TimeoutExpired:
                    with contextlib.suppress(ProcessLookupError):
                        os.killpg(process.pid, signal.SIGKILL)
                    process.wait()


def _unlock(runner, data, ui, prompt):
    protected = data is not None and textmerge.keyring_is_encrypted(data)
    if protected and not ui.interactive:
        raise KeyringError("A selected keyring needs a password. Run interactively without --yes "
                           "or --json, or leave browser profiles and keys out of this import.")
    for _ in range(3):
        password = ui.password(prompt) if protected else ""
        try:
            return _operate(runner, data, password), password
        except Locked:
            ui.warn("That password did not unlock the keyring. Try again.")
    raise KeyringError("The keyring is still locked. Nothing was imported.")


def prepare(plan, destination, ui, runner):
    """Finish password-dependent decisions before any destination writes."""
    for action in plan.actions:
        if action.action != "unlock-keyring":
            continue
        try:
            source = safefs.source_read(action.source, textmerge.TEXT_LIMIT)
            current_info = destination.lstat(action.relative)
            current = destination.read(action.relative, textmerge.TEXT_LIMIT) if current_info else None
        except safefs.UnsafePath:
            raise KeyringError("A keyring path changed or became unsafe. Nothing was imported.") from None
        if source != action.content:
            raise KeyringError("The trial keyring changed while importing. Run the import again.")
        if (safefs.Fingerprint.of(current_info) if current_info else None) != action.expected:
            raise KeyringError("This computer's keyring changed while importing. Run the import again.")
        items, source_password = _unlock(runner, action.content, ui,
                                         "Trial keyring password (Ctrl+C cancels): ")
        if current is None:
            # A new keyring keeps the source password and its protection.
            target_password = source_password
        else:
            _, target_password = _unlock(runner, current, ui,
                                         "This computer's keyring password (Ctrl+C cancels): ")
        merged = _operate(runner, current, target_password, items, plan.keyring_apps)
        protected = current if current is not None else action.content
        if textmerge.keyring_is_encrypted(protected) and \
                not textmerge.keyring_is_encrypted(merged):
            raise KeyringError("The destination keyring's password protection could not be kept.")
        action.action = "same" if merged == current else ("merge" if current else "create")
        action.content = merged
        action.size = len(merged)
        action.backup = current is not None and merged != current
        action.reason = "keyring entries combined; unrelated entries on this computer are kept"


def _worker():
    """One private bus, one disposable keyring, no desktop unlock prompts."""
    resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
    try:
        import gi
        gi.require_version("Secret", "1")
        from gi.repository import Gio, GLib, Secret
    except (ImportError, ValueError):
        print(json.dumps({"error": "dependencies"}))
        return
    daemon = None
    try:
        request = json.load(sys.stdin)
        path = Path(os.environ["XDG_DATA_HOME"]) / "keyrings/login.keyring"
        original = path.read_bytes() if request["existing"] else None
        daemon = subprocess.Popen([request["daemon"], "--foreground", "--unlock", "--components=secrets"],
                                  stdin=subprocess.PIPE, stdout=subprocess.DEVNULL,
                                  stderr=subprocess.DEVNULL)
        daemon.stdin.write(request["password"].encode())
        daemon.stdin.close()
        connection = Gio.bus_get_sync(Gio.BusType.SESSION, None)
        deadline = time.monotonic() + 5
        while True:
            owned = connection.call_sync("org.freedesktop.DBus", "/org/freedesktop/DBus",
                "org.freedesktop.DBus", "NameHasOwner", GLib.Variant("(s)", ("org.freedesktop.secrets",)),
                None, Gio.DBusCallFlags.NONE, 1000, None).unpack()[0]
            if owned:
                break
            if daemon.poll() is not None or time.monotonic() > deadline:
                raise KeyringError()
            time.sleep(0.02)
        service = Secret.Service.get_sync(Secret.ServiceFlags.OPEN_SESSION | Secret.ServiceFlags.LOAD_COLLECTIONS, None)
        collection = Secret.Collection.for_alias_sync(service, "default", Secret.CollectionFlags.LOAD_ITEMS, None)
        if collection is None:
            raise KeyringError()
        locked = collection.get_locked()
        if locked and original is not None and not textmerge.keyring_is_encrypted(original):
            # --unlock skips an empty password. Secret Service can unlock a
            # plaintext collection directly, without displaying a prompt.
            unlocked, prompt = connection.call_sync("org.freedesktop.secrets", "/org/freedesktop/secrets",
                "org.freedesktop.Secret.Service", "Unlock", GLib.Variant("(ao)",
                ([collection.get_object_path()],)), None, Gio.DBusCallFlags.NONE, 5000, None).unpack()
            if prompt != "/" or collection.get_object_path() not in unlocked:
                raise KeyringError()
            collection.load_items_sync(None)
            locked = False
        if locked:
            print(json.dumps({"error": "locked"}))
            return
        def read(item):
            item.load_secret_sync(None)
            value = item.get_secret()
            return {"label": item.get_label(), "attributes": item.get_attributes(),
                    "value": base64.b64encode(bytes(value.get())).decode(),
                    "type": value.get_content_type()}
        existing = [read(item) for item in collection.get_items()]
        if request["items"] is None:
            if path.read_bytes() != original:
                raise KeyringError()
            print(json.dumps({"items": existing}))
            return
        by_identity = {(item["label"], tuple(sorted(item["attributes"].items()))): item for item in existing}
        changed = False
        # The D-Bus Secret struct accepts arbitrary bytes. GI's Secret.Value.new
        # only accepts text, which would lose binary credentials.
        session = connection.call_sync("org.freedesktop.secrets", "/org/freedesktop/secrets",
            "org.freedesktop.Secret.Service", "OpenSession", GLib.Variant("(sv)",
            ("plain", GLib.Variant("s", ""))), None, Gio.DBusCallFlags.NONE, 5000, None).unpack()[1]
        for item in request["items"]:
            identity = (item["label"], tuple(sorted(item["attributes"].items())))
            previous = by_identity.get(identity)
            if previous is not None and (previous == item or
                    item["attributes"].get("application") not in request["trial_wins"]):
                continue
            raw = base64.b64decode(item["value"], validate=True)
            secret = (session, [], raw, item["type"])
            # Match the identity explicitly; REPLACE only compares attributes.
            if previous is not None:
                for target in collection.get_items():
                    if target.get_label() == item["label"] and target.get_attributes() == item["attributes"]:
                        connection.call_sync("org.freedesktop.secrets", target.get_object_path(),
                            "org.freedesktop.Secret.Item", "SetSecret", GLib.Variant("((oayays))", (secret,)),
                            None, Gio.DBusCallFlags.NONE, 5000, None)
                        break
            else:
                properties = {"org.freedesktop.Secret.Item.Label": GLib.Variant("s", item["label"]),
                              "org.freedesktop.Secret.Item.Attributes": GLib.Variant("a{ss}", item["attributes"])}
                _, prompt = connection.call_sync("org.freedesktop.secrets", collection.get_object_path(),
                    "org.freedesktop.Secret.Collection", "CreateItem",
                    GLib.Variant("(a{sv}(oayays)b)", (properties, secret, False)),
                    None, Gio.DBusCallFlags.NONE, 5000, None).unpack()
                if prompt != "/":
                    raise KeyringError()
            by_identity[identity] = item
            changed = True
        data = path.read_bytes() if changed or original is None else original
        print(json.dumps({"data": base64.b64encode(data).decode()}))
    except Exception:
        # Never include a native error, item label, password or secret in diagnostics.
        print(json.dumps({"error": "operation"}))
    finally:
        if daemon is not None:
            daemon.terminate()
            try:
                daemon.wait(timeout=2)
            except subprocess.TimeoutExpired:
                daemon.kill()
                daemon.wait()
