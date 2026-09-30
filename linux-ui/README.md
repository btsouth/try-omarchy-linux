# Linux setup window

`try-omarchy-setup` is a GTK4/libadwaita helper with its own Go module. Install
it beside `try-omarchy`. The launcher remains cgo-free and its tests do not
import GTK. Ordinary desktop starts open a pre-boot home. `-start` and other
explicit CLI options start directly; `-launcher` forces the home for a scripted
launch. `-no-gui` keeps terminal-only operation; a missing display or helper
falls back to terminal status.

Build in the GNOME 50 SDK:

```sh
go build -tags 'gtk_4_18,adw_1_7' -o try-omarchy-setup .
```

The Flatpak builds both programs offline. After updating dependencies, run
`python3 runtime-build/linux/generate-go-sources.py` from the repository root
and review the generated source hashes along with `go.mod` and `go.sum`.

## Pipe protocol

Standard input carries newline-delimited JSON snapshots:

```json
{"status":"Downloading the Omarchy system...","current":42,"total":100,"error":false}
```

Status is plain text. A zero total means indeterminate progress. An error
replaces progress with a Close button. Standard output carries one JSON event
per line: `ready`, `cancel`, `dismissed`, or `reply`. Diagnostics go to standard error.
Closing standard input closes the helper, including when the launcher dies.

Cancel and the titlebar close button request cancellation once and wait for
the launcher to finish. Existing disks and downloaded files are retained.
An unexpected helper exit after readiness also cancels setup. Progress updates
are coalesced so a slow helper cannot stall downloads or disk writes.

The launcher closes the setup window when QEMU answers on its control socket.
This is a process readiness check, not proof that the guest desktop has loaded.
First-run snapshots use `prompt: "location"` (with the default `path`) or
`prompt: "account"`, and a monotonically increasing `request` number. Replies
carry the same request number and a `value`: `default` or the picked parent
folder, and `instant` or `personal`, respectively. GTK uses the native folder
portal in Flatpak. The launcher validates access before saving the location.
Explicit `-dir` and `-instant` flags take precedence over saved choices.

The idle home uses `prompt: "home"` with a read-only storage status and replies
`launch`, `settings`, `about`, or `close`. Its close reply exits before KVM,
downloads or disk creation. `settings` uses a structured form with automatic
RAM/CPU choices and rendering modes; Save sends a JSON value, Cancel sends
`cancel`. The launcher preserves settings fields absent from that form. Running
VM changes apply after a full shutdown and new QEMU launch; `settings-saved`
holds the feedback until Done. `about` shows the private Linux preview version
and basic help. The form and home content scroll above a fixed action footer.

After handoff, a new helper handles `prompt: "close"`. Its default action and
titlebar close return `keep`; only `shutdown` requests an ACPI powerdown.
Dismissal, helper failure, and repeated close requests never force-stop the VM.
`-no-gui` retains immediate ACPI shutdown on window close. A second terminal
interrupt remains the explicit force-stop action.

Run GUI checks in an isolated session or VM. Do not connect a test to the host
audio or session bus.

The branded windows share the Mac launcher's Tokyo Night palette and official
mark. GTK controls, folder pickers and keyboard navigation remain native. High
contrast removes the branded color overrides, including when changed live.
The launcher shows its Linux version and secondary actions wrap beneath its
primary action. Recovery actions scroll above the fixed Back button.

Launcher, account setup, progress, Settings, About and recovery use the same
mark, horizontal product heading and monospaced type hierarchy. Saved resource
and integration cards stay visible on the home screen; the longer first-use
explanation expands under What setup does. Automatic start only counts down
for an existing VM with no startup notice, so first setup and host problems
keep their explanation and actions visible.

Native layout checks inspect the real window and are opt-in:

```sh
TRYOMARCHY_UI_TEST=1 go test -tags 'gtk_4_18,adw_1_7' -run Native -v .
```

Run these inside an isolated desktop on a test machine. Check smaller windows,
larger desktop fonts and high contrast as well as the default size. Ordinary
headless tests explicitly skip native layout checks.

Settings groups resources, storage/shared files, display/keyboard, devices and
startup into cards. Advanced network settings expand separately. Balanced,
Maximum performance and Manual use the launcher's existing resource preference
format; switching profiles retains manual CPU/memory values. Numeric tuning is
shown for Manual. The Balanced summary is an estimate checked again at launch,
not a reservation or the running VM's allocation. Device refresh preserves
unsaved edits and the hidden manual values.

Save feedback distinguishes independently persisted groups and live audio
acknowledgement. Failed persistence leaves the form open with its edits and
identifies earlier successful writes. Startup follows the next app open; VM
configuration follows the next VM launch. A guest reboot does not restart the
host VM process.

Use the desktop's text-scaling setting for 150%/200% checks and confirm the
resulting rendered text, not only the requested window size. Native bounds
checks include horizontal overflow as well as the fixed action footer.

`TestNativeMultilineEnter` also accepts `TRYOMARCHY_UI_KEYBOARD_READY` naming a
private temporary marker file. It focuses the real network text field and
writes the marker; send Return through the isolated compositor. The test checks
that a newline is inserted without submitting Settings. Without external key
automation, this check is explicitly skipped.
