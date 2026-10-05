//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// superviseLinux runs QEMU until the guest powers off. KVM resets in place,
// so a guest reboot stays inside one QEMU and one window; QEMU is only
// relaunched for the startup fallbacks: audio, memory, then CPU rendering.
func superviseLinux(cfg *config, cmdline string, stop <-chan os.Signal) error {
	const maxLaunchAttempts = 8
	for attempt := 1; attempt <= maxLaunchAttempts; attempt++ {
		if err := checkSetupCancelled(); err != nil {
			fatalf("%v", err)
		}
		getUI().setBooting(true)
		mode := "CPU rendering (llvmpipe)"
		if cfg.useGpu {
			mode = "GPU accelerated (virgl + Venus Vulkan)"
			if !linuxVenusEnabled {
				mode = "GPU accelerated OpenGL (software Vulkan)"
			}
		}
		getUI().setCatalogStatus("status.linux.starting", map[string]string{"mode": mode})
		logf("booting - %s (attempt %d)", mode, attempt)
		controlDir, err := prepareQMPControl()
		if err != nil {
			fatal(uiTextWith("fatal.vm_controls", map[string]string{"error": fmt.Sprintf("%v", err)}))
		}
		cfg.qmpDir = controlDir
		// A startup fallback relaunch keeps forwards Settings changed live.
		cfg.forwards = forwardsForBoot(cfg.launchForwards)
		guestReady.Store(false)
		desktopReady.Store(false)
		args := linuxQemuArgs(cfg, buildQemuArgs(cfg, cmdline))
		logf("qemu: %s %s", cfg.qemu, strings.Join(args, " "))
		proc := exec.Command(cfg.qemu, args...)
		proc.Env = linuxDisplayEnvironment(linuxQemuEnvironment(os.Environ()), cfg)
		// Per attempt: the fallbacks read this attempt's errors only.
		stderr, err := os.OpenFile(filepath.Join(cfg.vmDir, "qemu-stderr.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err == nil {
			proc.Stdout, proc.Stderr = stderr, stderr
		}
		if err := proc.Start(); err != nil {
			fatal(uiTextWith("fatal.qemu.start", map[string]string{"error": fmt.Sprintf("%v", err)}))
		}
		exited := make(chan error, 1)
		vmDone := make(chan struct{})
		go func() {
			err := proc.Wait()
			close(vmDone)
			if err != nil {
				logf("QEMU process exited with error: %v", err)
			}
			exited <- err
		}()

		qmp, died := connectLinuxQMP(exited)
		if qmp != nil {
			// Once per QEMU launch, independent of guest readiness and resets.
			go startLinuxBootUSB(setupContext(), cfg.dir, vmDone)
			stopPower := startLinuxPower(vmDone)
			defer stopPower()
			lines := qmp.readLines()
			visibility := &linuxVisibility{}
			initialInterrupts := 0
			desktopTimedOut := false
			confirmation := newLinuxShutdownConfirmation(confirmLinuxShutdown)
			defer confirmation.close()
			if setupCancelled() {
				requestLinuxShutdown(qmp, proc, &initialInterrupts)
				getUI().finish()
			}
			if linuxGUIEnabled && initialInterrupts == 0 {
				getUI().setCatalogStatus("status.linux.booting", nil)
				result := waitLinuxDesktopReady(setupContext(), exited, stop, lines, visibility,
					desktopReady.Load, guestReady.Load, linuxDesktopReadyTimeout, 250*time.Millisecond,
					func(_ string) { getUI().setCatalogStatus("status.linux.desktop_starting", nil) }, confirmation)
				switch result {
				case linuxDesktopReady:
					getUI().finish()
					if linuxGUIEnabled {
						go showLinuxSessionTips(cfg.instant)
					}
				case linuxDesktopTimedOut:
					desktopTimedOut = true
					getUI().showDesktopTimeout(uiText("shutdown.linux.omarchy_is_running_finish_account_setup_or_sign"))
				case linuxDesktopCancelled:
					confirmation.close()
					requestLinuxShutdown(qmp, proc, &initialInterrupts)
					getUI().finish()
				case linuxDesktopInterrupted:
					requestLinuxShutdown(qmp, proc, &initialInterrupts)
					getUI().finish()
				case linuxDesktopExited:
					stopPower()
					confirmation.close()
					qmp.close()
					if stderr != nil {
						stderr.Close()
					}
					fatal(uiTextWith("shutdown.linux.omarchy_stopped_before_its_desktop_became_ready_check", map[string]string{"error": filepath.Join(cfg.vmDir, "qemu-stderr.log")}))
				}
			} else if initialInterrupts == 0 {
				getUI().finish()
			}
			err := watchLinux(cfg, qmp, proc, exited, stop, lines, visibility, initialInterrupts, confirmation, desktopTimedOut)
			stopPower()
			qmp.close()
			if stderr != nil {
				stderr.Close()
			}
			return err
		}
		if !died {
			logf("QEMU did not answer on its control socket - stopping it")
			proc.Process.Kill()
			<-exited
			fatal(uiTextWith("shutdown.linux.qemu_did_not_start_answering_see_s", map[string]string{"error": filepath.Join(cfg.vmDir, "qemu-stderr.log")}))
		}
		if stderr != nil {
			stderr.Close()
		}
		if detail := qemuStartupFailureTail(cfg.vmDir); detail != "" {
			logf("QEMU startup failure (attempt %d, %s):\n%s", attempt, mode, detail)
		}
		if !linuxStartupFallback(cfg) {
			fatal(uiTextWith("shutdown.linux.qemu_exited_at_startup_see_s", map[string]string{"error": filepath.Join(cfg.vmDir, "qemu-stderr.log")}))
		}
	}
	fatal(uiTextWith("shutdown.linux.qemu_failed_to_come_up_after_d_attempts", map[string]string{"count": fmt.Sprintf("%d", maxLaunchAttempts)}))
	return nil
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
func watchLinux(cfg *config, qmp *qmpConn, proc *exec.Cmd, exited <-chan error, stop <-chan os.Signal,
	lines <-chan string, visibility *linuxVisibility, interrupts int, confirmation *linuxShutdownConfirmation, desktopTimedOut bool) error {
	logf("supervisor: watching guest lifecycle")
	if interrupts > 0 {
		confirmation.shutdownAt = time.Now()
	}
	requestShutdown := func() {
		if interrupts == 0 {
			confirmation.shutdownAt = time.Now()
		}
		requestLinuxShutdown(qmp, proc, &interrupts)
	}
	if getUI().window == nil && interrupts == 0 {
		getUI().setStatus("%s", uiText("shutdown.linux.omarchy_is_running_close_its_window_shut_it"))
	}
	reason := ""
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopTray := startLinuxTray()
	defer stopTray()
	leaseTicker := time.NewTicker(5 * time.Second)
	defer leaseTicker.Stop()
	shutdownRetry := time.NewTicker(linuxShutdownRetryInterval)
	defer shutdownRetry.Stop()
	var startupStop <-chan struct{}
	if desktopTimedOut {
		startupStop = setupCancelWake
	}
	graphicsWarningShown := false
	imageConfirmed := false
	defer func() { visibility.visible = false; sendLinuxVisibility(visibility) }()
	for {
		if !imageConfirmed && guestReady.Load() {
			recordRenderResult(cfg)
			commitGuestPayloadUpdate(cfg.dir)
			commitCheckpointBoot(cfg.dir)
			markLinuxMovedGuestReady(defaultLinuxDataDirectory(), cfg.dir)
			imageConfirmed = true
		}
		if desktopTimedOut && desktopReady.Load() {
			logf("guest desktop appeared after startup timeout")
			getUI().finish()
			// finish waits for the helper to exit, consuming any Stop reply
			// sent just before desktop readiness closed the waiting window.
			if setupCancelled() {
				confirmation.close()
				if interrupts == 0 {
					requestShutdown()
				}
			} else if linuxGUIEnabled {
				go showLinuxSessionTips(cfg.instant)
			}
			desktopTimedOut = false
			startupStop = nil
		}
		select {
		case err := <-exited:
			confirmation.close()
			if reason == "" {
				reason = "QEMU exited"
			}
			if err != nil && interrupts == 0 {
				return uiError(uiTextWith("shutdown.linux.omarchy_stopped_unexpectedly_check_for_the_vm_error", map[string]string{"path": filepath.Join(cfg.vmDir, "qemu-stderr.log"), "error": fmt.Sprint(err)}), err)
			}
			logf("guest stopped (%s)", reason)
			getUI().setStatus("%s", uiText("shutdown.linux.omarchy_stopped"))
			return nil
		case line, ok := <-lines:
			if !ok {
				lines = nil
				continue
			}
			if visibility.receive(line, time.Now()) {
				sendLinuxVisibility(visibility)
			}
			if r := shutdownReason(line); r != "" {
				reason = r
			}
			if closeRequested(line) {
				logf("window close requested")
				if !linuxGUIEnabled {
					if interrupts == 0 || time.Since(confirmation.shutdownAt) >= linuxShutdownGracePeriod {
						requestShutdown()
					}
				} else {
					confirmation.requestShutdown(interrupts, time.Now())
				}
			}
			if linuxDropPointerMoved(line) {
				dropPointerMoves.Add(1)
			}
			if paths, point, ok := droppedFilesEvent(line); ok {
				// Time the drop from QEMU's own event timestamp, when its
				// pointer watch started, not from when this loop got to it.
				position := linuxDropPosition(line)
				at, timed := linuxDropTime(line, time.Now())
				if !timed {
					position = nil
				}
				if position == nil && point != nil {
					position = point[:]
				}
				// Count moves from here: a grant prompt means the pointer
				// moves on before the files reach the guest.
				moves := dropPointerMoves.Load()
				go func() {
					granted, err := linuxGrantDroppedFiles(paths)
					if errors.Is(err, errSetupCancelled) {
						logf("file drop: cancelled")
						return
					}
					if err == nil {
						err = queueDroppedFiles(droppedFiles{paths: granted, point: position, pointerMoves: moves, at: at})
					}
					reportLinuxFileDropError(err)
				}()
			}
		case <-leaseTicker.C:
			sendLinuxVisibility(visibility)
			if cfg.useGpu && !graphicsWarningShown && linuxVirglDesktopError(cfg.vmDir) {
				graphicsWarningShown = true
				logf("graphics: virgl reported a guest display error; offering software rendering recovery")
				showLinuxRuntimeError(uiText("shutdown.linux.graphics_error"), uiText("shutdown.linux.the_vm_reported_a_graphics_error_if_omarchy"))
			}
		case <-shutdownRetry.C:
			if interrupts == 1 && guestReady.Load() {
				logf("guest userspace is ready; repeating ACPI shutdown request")
				if err := qmp.writeLine(`{"execute":"system_powerdown"}`); err != nil {
					logf("ACPI shutdown retry: %v", err)
				}
			}
		case <-linuxSettingsRequests:
			go showLinuxSettings(ctx, cfg.dir)
		case <-linuxReclaimRequests:
			go showLinuxReclaim(ctx, cfg.dir)
		case <-linuxShareRequests:
			go openLinuxSharedFolder(cfg.share)
		case <-linuxDiagnosticsRequests:
			go createLinuxDiagnostics(cfg.dir)
		case <-linuxUSBRequests:
			go showLinuxUSBDevices(ctx)
		case <-linuxHelpRequests:
			go showLinuxHelp(ctx)
		case <-linuxShutdownRequests:
			confirmation.requestShutdown(interrupts, time.Now())
		case confirmed := <-confirmation.pending:
			confirmation.pending = nil
			if confirmed && (interrupts == 0 || interrupts == 1 && confirmation.force) {
				requestShutdown()
			}
		case <-startupStop:
			startupStop = nil
			confirmation.close()
			if interrupts == 0 {
				requestShutdown()
			}
			getUI().finish()
		case <-stop:
			logf("interrupt")
			requestShutdown()
		}
	}
}

var linuxShutdownRetryInterval = 5 * time.Second

// requestLinuxShutdown presses the ACPI power button so the guest shuts
// down cleanly; a second request stops QEMU.
func requestLinuxShutdown(qmp *qmpConn, proc *exec.Cmd, requests *int) {
	*requests++
	command := `{"execute":"system_powerdown"}`
	if *requests == 1 {
		getUI().setStatus("%s", uiText("launcher.linux.asking_omarchy_to_shut_down"))
	} else {
		logf("second shutdown request: stopping QEMU")
		command = `{"execute":"quit"}`
	}
	if err := qmp.writeLine(command); err != nil {
		proc.Process.Kill()
	}
}

// closeRequested matches the runtime's DISPLAY_CLOSE_REQUEST event, sent
// when the window's close button is used (window-close=off).
func closeRequested(line string) bool {
	var event struct {
		Event string `json:"event"`
	}
	return json.Unmarshal([]byte(line), &event) == nil && event.Event == "DISPLAY_CLOSE_REQUEST"
}

// linuxQemuEnvironment names QEMU's window after the app, so desktops group
// it with the launcher and its icon, and turns on the runtime's desktop
// behavior: the window title is the app name and the keyboard (the Super key
// included) goes to the guest while the window has focus.
func linuxQemuEnvironment(env []string) []string {
	clean := make([]string, 0, len(env)+8)
	for _, value := range env {
		if !strings.HasPrefix(value, "QEMU_SDL_GUEST_SCALE=") {
			clean = append(clean, value)
		}
	}
	env = clean
	if linuxGuestScale != "auto" && linuxGuestScale != "keep" && linuxGuestScale != "" {
		env = append(env, "QEMU_SDL_GUEST_SCALE="+linuxGuestScale)
	}
	env = append(env,
		// The runtime uses SDL2-compat. Its default hides fractional scaling
		// from older applications; our backing-pixel bridge handles it.
		"SDL_VIDEO_WAYLAND_SCALE_TO_DISPLAY=0",
		"SDL_VIDEO_WAYLAND_WMCLASS="+linuxAppID,
		"SDL_VIDEO_X11_WMCLASS="+linuxAppID,
		"QEMU_SDL_TITLE_FROM_NAME=1",
		"QEMU_SDL_FOCUS_KEYBOARD_GRAB=1",
	)
	if inFlatpak() {
		// The runtime lists each Vulkan driver in two directories, so every
		// host GPU reached Venus twice, and its llvmpipe made guest apps
		// render on the host CPU by default.
		env = append(env,
			"VK_DRIVER_FILES=/usr/lib/x86_64-linux-gnu/GL/vulkan/icd.d",
			"VK_LOADER_DRIVERS_DISABLE=*lvp*",
		)
	}
	return env
}

func inFlatpak() bool {
	_, err := os.Stat("/.flatpak-info")
	return err == nil
}

const linuxAppID = "com.tryomarchy.TryOmarchy"

var linuxGUIEnabled bool

func confirmLinuxShutdown(parent context.Context) <-chan bool {
	result := make(chan bool, 1)
	go func() {
		ctx, cancel := context.WithCancel(parent)
		defer cancel()
		w := startLinuxWindow(cancel)
		if w == nil {
			logf("Could not open shutdown confirmation; Omarchy is still running. Shut down from its own menu or press Ctrl+C in the terminal.")
			result <- false
			return
		}
		defer w.stop()
		answer, err := w.ask(ctx, linuxSetupState{Prompt: "close"})
		result <- err == nil && answer == "shutdown"
	}()
	return result
}

func confirmLinuxForceStop(parent context.Context) <-chan bool {
	result := make(chan bool, 1)
	go func() {
		ctx, cancel := context.WithCancel(parent)
		defer cancel()
		w := startLinuxWindow(cancel)
		if w == nil {
			logf("Could not open force stop confirmation; Omarchy is still running. Press Ctrl+C in the terminal to stop it.")
			result <- false
			return
		}
		defer w.stop()
		answer, err := w.ask(ctx, linuxForceStopState())
		result <- err == nil && answer == "secondary"
	}()
	return result
}

func linuxForceStopState() linuxSetupState {
	return linuxSetupState{Prompt: "choice", Title: uiText("shutdown.linux.force_stop_omarchy"), Primary: uiText("shutdown.linux.keep_waiting"), Secondary: uiText("shutdown.linux.force_stop"), Destructive: true,
		Status: uiText("shutdown.linux.omarchy_has_not_shut_down_yet_force_stopping")}
}
