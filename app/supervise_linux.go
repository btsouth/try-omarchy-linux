//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// superviseLinux runs QEMU until the guest powers off. KVM resets in place,
// so a guest reboot stays inside one QEMU and one window; QEMU is only
// relaunched for the startup fallbacks: audio, memory, then CPU rendering.
func superviseLinux(cfg *config, cmdline string, stop <-chan os.Signal) {
	const maxLaunchAttempts = 8
	for attempt := 1; attempt <= maxLaunchAttempts; attempt++ {
		mode := "CPU rendering (llvmpipe)"
		if cfg.useGpu {
			mode = "GPU accelerated (virgl + Venus Vulkan)"
		}
		getUI().setStatus("Starting Omarchy - %s", mode)
		logf("booting - %s (attempt %d)", mode, attempt)
		controlDir, err := prepareQMPControl()
		if err != nil {
			fatal("Cannot prepare private VM controls: %v", err)
		}
		cfg.qmpDir = controlDir
		args := buildQemuArgs(cfg, cmdline)
		logf("qemu: %s %s", cfg.qemu, strings.Join(args, " "))
		proc := exec.Command(cfg.qemu, args...)
		proc.Env = linuxQemuEnvironment(os.Environ())
		// Per attempt: the fallbacks read this attempt's errors only.
		stderr, err := os.OpenFile(filepath.Join(cfg.vmDir, "qemu-stderr.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err == nil {
			proc.Stdout, proc.Stderr = stderr, stderr
		}
		if err := proc.Start(); err != nil {
			fatal("QEMU failed to start: %v", err)
		}
		exited := make(chan error, 1)
		go func() {
			err := proc.Wait()
			if err != nil {
				logf("QEMU process exited with error: %v", err)
			}
			exited <- err
		}()

		qmp, died := connectLinuxQMP(exited)
		if qmp != nil {
			watchLinux(qmp, proc, exited, stop)
			qmp.close()
			if stderr != nil {
				stderr.Close()
			}
			return
		}
		if !died {
			logf("QEMU did not answer on its control socket - stopping it")
			proc.Process.Kill()
			<-exited
			fatal("QEMU did not start answering - see %s.", filepath.Join(cfg.vmDir, "qemu-stderr.log"))
		}
		if stderr != nil {
			stderr.Close()
		}
		if detail := qemuStartupFailureTail(cfg.vmDir); detail != "" {
			logf("QEMU startup failure (attempt %d, %s):\n%s", attempt, mode, detail)
		}
		if !linuxStartupFallback(cfg) {
			fatal("QEMU exited at startup - see %s.", filepath.Join(cfg.vmDir, "qemu-stderr.log"))
		}
	}
	fatal("QEMU failed to come up after %d attempts.", maxLaunchAttempts)
}

// KVM has no launch wedge, so the control socket is tried as soon as QEMU
// has had a moment to create it. died reports that QEMU exited first.
func connectLinuxQMP(exited <-chan error) (qmp *qmpConn, died bool) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			return nil, true
		case <-time.After(500 * time.Millisecond):
			if qmp = qmpConnect(qmpSupPort, 5*time.Second); qmp != nil {
				return qmp, false
			}
		}
	}
	return nil, false
}

// linuxStartupFallback changes one thing for the next attempt, in the same
// order as Windows: silent audio, less memory, then CPU rendering.
func linuxStartupFallback(cfg *config) bool {
	if cfg.audio != "none" && linuxAudioUnavailable(cfg) {
		cfg.audio = "none"
		logf("QEMU exited at startup - retrying without audio")
		return true
	}
	if memoryStarved(cfg) && cfg.memMiB > 1024 {
		cfg.memMiB = max(cfg.memMiB/2, 1024)
		logf("QEMU exited at startup - low memory, retrying with %d MiB", cfg.memMiB)
		return true
	}
	if cfg.useGpu {
		cfg.useGpu = false
		logf("QEMU exited at startup - falling back to CPU rendering")
		return true
	}
	return false
}

func linuxAudioUnavailable(cfg *config) bool {
	data, err := os.ReadFile(filepath.Join(cfg.vmDir, "qemu-stderr.log"))
	if err != nil {
		return false
	}
	text := strings.ToLower(string(data))
	for _, marker := range []string{"pipewire", "audio: could not init", "failed to initialize audio"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// watchLinux follows the guest until QEMU exits. A first interrupt presses
// the ACPI power button so the guest shuts down cleanly; another one quits.
func watchLinux(qmp *qmpConn, proc *exec.Cmd, exited <-chan error, stop <-chan os.Signal) {
	logf("supervisor: watching guest lifecycle")
	getUI().setStatus("Omarchy is running. Shut it down from its own menu, or press Ctrl+C here.")
	lines := qmp.readLines()
	reason := ""
	interrupts := 0
	for {
		select {
		case <-exited:
			if reason == "" {
				reason = "QEMU exited"
			}
			logf("guest stopped (%s)", reason)
			getUI().setStatus("Omarchy stopped.")
			return
		case line, ok := <-lines:
			if !ok {
				lines = nil
				continue
			}
			if r := shutdownReason(line); r != "" {
				reason = r
			}
		case <-stop:
			interrupts++
			if interrupts == 1 {
				getUI().setStatus("Asking Omarchy to shut down...")
				logf("interrupt: requesting guest poweroff")
				if err := qmp.writeLine(`{"execute":"system_powerdown"}`); err != nil {
					proc.Process.Kill()
				}
			} else {
				logf("second interrupt: stopping QEMU")
				if err := qmp.writeLine(`{"execute":"quit"}`); err != nil {
					proc.Process.Kill()
				}
			}
		}
	}
}

// linuxQemuEnvironment names QEMU's window after the app, so desktops group
// it with the launcher and its icon.
func linuxQemuEnvironment(env []string) []string {
	return append(env,
		"SDL_VIDEO_WAYLAND_WMCLASS="+linuxAppID,
		"SDL_VIDEO_X11_WMCLASS="+linuxAppID,
	)
}

const linuxAppID = "com.tryomarchy.TryOmarchy"
