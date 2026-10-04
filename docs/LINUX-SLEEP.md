# Linux sleep handling

On hosts with systemd-logind, Try Omarchy listens for `PrepareForSleep` on
`org.freedesktop.login1` through the system bus. While the VM runs, it holds a
[sleep delay inhibitor](https://systemd.io/INHIBITOR_LOCKS/). When the host
prepares to sleep, the launcher pauses a running VM and releases the inhibitor
so sleep can proceed. After wake, it resumes only the pause it still owns and
takes a new inhibitor for the next sleep. The Flatpak allows communication with
`org.freedesktop.login1` for this purpose.

Power handling uses a dedicated private QMP connection. It preserves a manual
pause, shutdown, restore or guest suspend. Manual STOP or RESUME events during
sleep give up automatic resume ownership. Duplicate notifications do not repeat
successful work, and a replacement VM cannot inherit an old resume obligation.
The monitor starts when the supervisor's controls answer, before the launcher
waits for the guest desktop, and closes when that VM exits or supervision ends.

QMP commands have a two-second budget per transition. The inhibitor is released
even when pausing fails. A disconnected connection, lost reply or unconfirmed
pause clears ownership; the VM may remain paused and need a manual resume.
A rejected resume command can be retried by a later wake notification while the
original connection still proves ownership. Closing the launcher releases its
inhibitor and connections without automatically resuming an uncertain pause.
If logind or its inhibitor is unavailable, the launcher logs the failure once
for that VM and continues without sleep handling.

After wake, the existing guest agent sends the host time immediately and again
five seconds later. This is the same clock correction hook Windows uses. The
shared guest image also disables service watchdogs that could fire after a long
host sleep; that is separate from clock correction.

## Verification limits

Unit tests cover repeated and duplicate notifications, non-running guests,
manual changes during sleep, replacement runtimes, command failures, inhibitor
release and cleanup. A private D-Bus fixture checks fake login1 signals and
Unix file descriptor transfer without contacting the real system bus. It skips
when `dbus-daemon` is missing. A diskless QEMU fixture checks actual pause/resume
states and independent controls while paused when `QEMU_SYSTEM` is set.

These tests do not suspend a host. Physical suspend, hibernate, lid close,
Flatpak bus policy and guest desktop recovery still need checks on an installed
candidate. Check repeated sleep/wake cycles, a manual pause before sleep,
manual resume/pause during the interval, clock correction, and shutdown during
a sleep cycle on a suitable test machine.
