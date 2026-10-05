//go:build linux

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// Try Omarchy for Linux. It prepares the guest the same way the Windows
// launcher does and runs it with KVM in QEMU's SDL window. A separate GTK
// process shows setup progress; the launcher itself remains cgo-free.

// failLinuxSetup reports a setup failure in plain words with a Try again that
// restarts setup from what is on disk: downloads continue where they stopped.
func failLinuxSetup(err error, dir string) {
	if setupCancelled() || errors.Is(err, errSetupCancelled) {
		fatalf("%v", err)
	}
	logf("FATAL setup failed: %v", err)
	fmt.Fprintf(os.Stderr, "%s: setup failed: %v\n", appTitle, err)
	if getUI().showFailure(classifyLinuxSetupFailure(err, dir)) {
		getUI().finish()
		relaunchLinuxSelf()
	}
	getUI().finish()
	os.Exit(1)
}

const linuxRetryEnv = "TRY_OMARCHY_RETRY"

// relaunchLinuxSelf starts setup again in this process. Everything that
// mattered was saved on disk (the location, the account choice, partial
// downloads), so the new run skips the questions and picks up where this one
// stopped. It returns only if the program cannot be started.
func relaunchLinuxSelf() {
	exe, err := os.Executable()
	if err != nil {
		logf("retry: %v", err)
		return
	}
	os.Setenv(linuxRetryEnv, "1")
	logf("retrying setup")
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		logf("retry: %v", err)
	}
}

// fatal shows a finished message and exits. Shared code passes catalog text
// from uiText or uiTextWith; fatalf formats already-resolved errors.
func fatal(msg string) {
	if setupCancelled() {
		getUI().finish()
		logf("Setup cancelled; existing disks and downloaded files retained")
		os.Exit(0)
	}
	logf("FATAL %s", msg)
	fmt.Fprintf(os.Stderr, "%s: %s\n", appTitle, msg)
	getUI().showError(msg)
	os.Exit(1)
}

func fatalf(format string, a ...any) { fatal(fmt.Sprintf(format, a...)) }

// Inside the Flatpak XDG_DATA_HOME is the app's own data folder.
func defaultLinuxDataDirectory() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "try-omarchy")
}

// main parses flags, shows the home unless launching directly, prepares the
// guest and runs it until it shuts down.
func main() {
	cfg := &config{}
	flag.StringVar(&cfg.dir, "dir", defaultLinuxDataDirectory(), uiText("launcher.linux.flag.dir"))
	flag.StringVar(&cfg.qemu, "qemu", "qemu-system-x86_64", uiText("launcher.linux.flag.qemu"))
	flag.StringVar(&cfg.share, "share", "", uiText("launcher.linux.flag.share"))
	chooseShare := flag.Bool("choose-share", false, uiText("launcher.linux.flag.choose_share"))
	flag.BoolVar(&cfg.fresh, "fresh", false, uiText("launcher.linux.flag.fresh"))
	flag.BoolVar(&cfg.fullscreen, "fullscreen", false, uiText("launcher.linux.flag.fullscreen"))
	flag.StringVar(&cfg.fullscreenDisplay, "fullscreen-display", "", uiText("launcher.linux.flag.fullscreen_display"))
	flag.IntVar(&cfg.memOverrideMiB, "memory", 0, uiText("launcher.linux.flag.memory"))
	flag.IntVar(&cfg.cpuOverride, "cpus", 0, uiText("launcher.linux.flag.cpus"))
	resourceProfileFlag := flag.String("resource-profile", "", uiText("launcher.linux.flag.resource_profile"))
	flag.IntVar(&cfg.diskGiB, "disk-size", 0, uiText("launcher.linux.flag.disk_size"))
	renderFlag := flag.String("render", "", uiText("launcher.linux.flag.render"))
	venusFlag := flag.String("venus", "auto", uiText("launcher.linux.flag.venus"))
	scaleFlag := flag.String("scale", "auto", uiText("launcher.linux.flag.scale"))
	audioFlag := flag.String("audio", "auto", uiText("launcher.linux.flag.audio"))
	audioOutput := flag.String("audio-output", "", uiText("launcher.linux.flag.audio_output"))
	audioInput := flag.String("audio-input", "", uiText("launcher.linux.flag.audio_input"))
	microphone := flag.Bool("microphone", true, uiText("launcher.linux.flag.microphone"))
	vulkanPresentFlag := flag.String("vulkan-present", "auto", uiText("launcher.linux.flag.vulkan_present"))
	timeZoneFlag := flag.String("timezone", "", uiText("launcher.linux.flag.timezone"))
	keyboardFlag := flag.String("keyboard", "", uiText("launcher.linux.flag.keyboard"))
	localeFlag := flag.String("locale", "", uiText("launcher.linux.flag.locale"))
	flag.BoolVar(&cfg.instant, "instant", false, uiText("launcher.linux.flag.instant"))
	var forwards forwardList
	flag.Var(&forwards, "forward", uiText("launcher.linux.flag.forward"))
	sshPort := flag.Int("ssh", 0, uiText("launcher.linux.flag.ssh"))
	sshKeyPath := flag.String("ssh-key", "", uiText("launcher.linux.flag.ssh_key"))
	width := flag.Int("width", 1280, uiText("launcher.linux.flag.width"))
	height := flag.Int("height", 800, uiText("launcher.linux.flag.height"))
	release := flag.String("release", linuxGuestReleaseURL, uiText("launcher.linux.flag.release"))
	sumsSHA256 := flag.String("sums-sha256", linuxGuestSumsSHA256, uiText("launcher.linux.flag.sums_sha256"))
	noGUI := flag.Bool("no-gui", false, uiText("launcher.linux.flag.no_gui"))
	startDirect := flag.Bool("start", false, uiText("launcher.linux.flag.start"))
	showLauncher := flag.Bool("launcher", false, uiText("launcher.linux.flag.launcher"))
	autostart := flag.Bool("autostart", false, uiText("launcher.linux.flag.autostart"))
	reclaim := flag.Bool("reclaim", false, uiText("launcher.linux.flag.reclaim"))
	flag.Parse()
	if *reclaim {
		// The running launcher owns the lifecycle port; this only talks to it.
		os.Exit(sendLinuxReclaim(fmt.Sprintf("127.0.0.1:%d", lifecyclePort), os.Stdout, os.Stderr))
	}
	if os.Getenv(linuxRetryEnv) != "" {
		// A retry continues the setup the person already chose.
		os.Unsetenv(linuxRetryEnv)
		*startDirect = true
		linuxQuickSetup.Store(true)
	}
	explicitFlags := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicitFlags[f.Name] = true })
	configureSetupCancellation(false)
	// Claim the instance before the idle home can save settings. An ordinary
	// desktop launch waits for an explicit Launch choice; CLI use stays direct.
	runLifecycleListener()
	if err := recoverLinuxMove(defaultLinuxDataDirectory()); err != nil {
		fatal(uiTextWith("fatal.linux.could_not_finish_moving_the_omarchy_data_folder", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	if explicitFlags["dir"] {
		resolved, err := resolveLinuxMovedDirectory(defaultLinuxDataDirectory(), cfg.dir)
		if err != nil {
			fatal(uiTextWith("settings.linux.cannot_read_the_data_folder_move_record", map[string]string{"error": fmt.Sprintf("%v", err)}))
		}
		cfg.dir = resolved
	}
	if !*noGUI && !*startDirect && (*showLauncher || *autostart || !linuxDirectStart(explicitFlags)) {
		if !showLinuxHome(defaultLinuxDataDirectory(), cfg.dir, explicitFlags["dir"], *autostart) {
			return
		}
	}
	if !*noGUI {
		getUI().startWindow()
	}
	defer getUI().finish()
	selectedRelease, selectedSumsSHA256, err := selectLinuxGuestRelease(*release, *sumsSHA256,
		explicitFlags["release"], explicitFlags["sums-sha256"])
	if err != nil {
		fatal(uiTextWith("fatal.linux.cannot_select_the_linux_omarchy_image_v", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}

	if err := checkKVM(); err != nil {
		fatalf("%v.", err)
	}
	qemu, err := exec.LookPath(cfg.qemu)
	if err != nil {
		fatal(uiTextWith("fatal.linux.cannot_find_qemu_s_v", map[string]string{"qemu": cfg.qemu, "error": fmt.Sprintf("%v", err)}))
	}
	cfg.qemu = qemu
	cfg.supportsSharing = true
	linuxGUIEnabled = !*noGUI
	var chooser dataLocationChooser
	if getUI().window != nil {
		chooser = getUI().chooseLocation
	}
	selected, proceed, err := resolveLinuxDataDirectory(defaultLinuxDataDirectory(), cfg.dir, explicitFlags["dir"], chooser)
	if err != nil {
		fatal(uiTextWith("fatal.linux.cannot_select_the_data_folder_v", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	if !proceed {
		return
	}
	cfg.dir = selected
	if !explicitFlags["dir"] && !pathsEqual(selected, defaultLinuxDataDirectory()) {
		noteLinuxLocationHint(defaultLinuxDataDirectory(), selected)
	}
	if cfg.dir, err = filepath.Abs(cfg.dir); err != nil {
		fatal(uiTextWith("fatal.linux.cannot_resolve_the_data_directory_v", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	cfg.hostDir = cfg.dir
	cfg.guestDir = filepath.Join(cfg.dir, "guest")
	cfg.vmDir = filepath.Join(cfg.dir, "vm")
	cfg.diskFormat = "raw"
	cfg.disk = filepath.Join(cfg.vmDir, "disk.raw")
	tightenLinuxGuestData(cfg.dir)
	// An interrupted roll back can have moved vm aside. Finish or undo it
	// before anything creates or reads the VM's files.
	if err := recoverLinuxSnapshots(cfg.dir); err != nil {
		fatal(uiTextWith("fatal.linux.could_not_finish_an_interrupted_snapshot_operation_v", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	if err := os.MkdirAll(cfg.vmDir, 0o700); err != nil {
		fatal(uiTextWith("fatal.data_directory", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	if shellLog, _ := os.OpenFile(filepath.Join(cfg.vmDir, "shell.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); shellLog != nil {
		openLog(shellLog)
	}
	logf("---- %s starting (Linux) ----", appTitle)
	if err := configureLinuxGraphics(*venusFlag); err != nil {
		fatalf("%v", err)
	}
	cfg.venus = linuxVenusEnabled
	if cfg.audio, err = linuxAudioMode(*audioFlag); err != nil {
		fatalf("%v", err)
	}

	if cfg.desktop, err = loadDesktopPreferences(cfg.dir); err != nil {
		fatal(uiTextWith("fatal.preferences.desktop", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	if cfg.audioDevices, err = loadAudioPreferences(cfg.dir); err != nil {
		fatal(uiTextWith("fatal.preferences.audio", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	if explicitFlags["audio-output"] {
		cfg.audioDevices.Output = *audioOutput
	}
	if explicitFlags["audio-input"] {
		cfg.audioDevices.Input = *audioInput
	}
	if explicitFlags["microphone"] {
		cfg.desktop.MicrophoneDisabled = !*microphone
	}
	if err := cfg.audioDevices.validate(); err != nil {
		fatal(uiTextWith("fatal.linux.cannot_use_audio_devices_v", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	experience, err := loadLinuxExperiencePreferences(cfg.dir)
	if err != nil {
		fatal(uiTextWith("fatal.linux.cannot_read_display_and_keyboard_preferences_v", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	if !explicitFlags["scale"] {
		*scaleFlag = experience.Scale
	}
	if !explicitFlags["keyboard"] {
		*keyboardFlag = experience.Keyboard
	}
	if linuxGuestScale, err = parseLinuxScale(*scaleFlag); err != nil {
		fatalf("%v", err)
	}
	userSettings, err := loadSettings(settingsPath(cfg.dir))
	if err != nil {
		fatal(uiTextWith("fatal.linux.cannot_read_its_settings_v_fix_or_delete", map[string]string{"error": fmt.Sprintf("%v", err), "path": settingsPath(cfg.dir)}))
	}
	if err := applySettings(cfg, userSettings, explicitFlags, &forwards, sshKeyPath); err != nil {
		fatal(uiTextWith("fatal.linux.cannot_use_its_settings_v", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	resourcePrefs, err := loadResourcePreferences(cfg.dir)
	if err != nil {
		fatal(uiTextWith("fatal.preferences.resources", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	if explicitFlags["resource-profile"] {
		resourcePrefs.Profile = *resourceProfileFlag
	}
	if err := validateResourceProfile(resourcePrefs.Profile); err != nil {
		fatalf("%v", err)
	}
	if explicitFlags["render"] {
		if cfg.renderMode, err = parseRenderMode(*renderFlag); err != nil {
			fatalf("%v", err)
		}
	}
	if cfg.memOverrideMiB != 0 && (cfg.memOverrideMiB < minimumGuestMemoryMiB || cfg.memOverrideMiB > maximumGuestMemoryMiB) {
		fatal(uiTextWith("fatal.linux.memory_must_be_between_d_and_d_mib", map[string]string{"minimum": fmt.Sprintf("%d", minimumGuestMemoryMiB), "maximum": fmt.Sprintf("%d", maximumGuestMemoryMiB)}))
	}
	if !explicitFlags["disk-size"] {
		storage, err := loadStorageSettings(cfg.dir)
		if err != nil {
			fatal(uiTextWith("fatal.preferences.storage", map[string]string{"error": fmt.Sprintf("%v", err)}))
		}
		cfg.diskGiB = storage.DiskGiB
	}
	if _, err := requestedDiskMiB(24*1024, cfg.diskGiB, false); err != nil {
		fatalf("%v", err)
	}
	home, _ := os.UserHomeDir()
	if cfg.sshKey, err = resolveSSHPreset(&forwards, *sshPort, *sshKeyPath, home, explicitFlags["ssh-key"]); err != nil {
		fatalf("%v.", err)
	}
	cfg.forwards = forwards
	cfg.launchForwards = append([]portForward(nil), cfg.forwards...)
	// Command-line -forward and -ssh replace the saved list for this launch.
	startLinuxLiveForwards(cfg.launchForwards, !explicitFlags["forward"] && !explicitFlags["ssh"])
	if len(userSettings.ForwardAdapters) > 0 {
		logf("LAN forwarding is not available on Linux yet; forwarding on loopback only")
	}

	var chooseAccount func() (string, error)
	if getUI().window != nil {
		chooseAccount = func() (string, error) {
			return getUI().window.ask(setupContext(), linuxAccountState())
		}
	}
	if err := chooseLinuxProvisionMode(cfg, explicitFlags["instant"], chooseAccount); err != nil {
		fatal(uiTextWith("fatal.linux.cannot_select_the_account_setup_v", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	// A shared folder is chosen in Settings. Asking at first setup put a third
	// question in front of the desktop, so only -choose-share asks it here.
	var chooseFolder func(string) (string, error)
	if getUI().window != nil && *chooseShare {
		chooseFolder = func(status string) (string, error) {
			return getUI().window.ask(setupContext(), linuxSetupState{Prompt: "share", Status: status})
		}
	}
	if *chooseShare && chooseFolder == nil {
		fatal(uiText("fatal.linux.choosing_a_shared_folder_requires_the_setup_window"))
	}
	if err := configureLinuxSharing(cfg, &userSettings, explicitFlags["share"], *chooseShare, chooseFolder); err != nil {
		fatal(uiTextWith("fatal.linux.cannot_configure_the_shared_folder_v", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	if cfg.share != "" {
		if cfg.share, err = validateLinuxSharedFolder(cfg.share, cfg.dir); err != nil {
			fatal(uiTextWith("fatal.linux.cannot_share_s_v", map[string]string{"path": cfg.share, "error": fmt.Sprintf("%v", err)}))
		}
	}
	if err := checkSetupCancelled(); err != nil {
		fatalf("%v", err)
	}

	var reason string
	reason = configureLinuxRender(cfg, linuxDisplayDriverIdentity())
	if reason != "" {
		logf("rendering: %s", reason)
	}

	if err := recoverLinuxGuestUpdate(cfg.dir, &selectedRelease, &selectedSumsSHA256); err != nil {
		fatal(uiTextWith("fatal.linux.could_not_restore_the_previous_omarchy_image_after", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	// A rolled-back VM first boots on the system files saved with it. A newer
	// image, if this app pins one, is fetched on the launch after that.
	var runtimeRelease, runtimeSums string
	if pinned, err := pinCheckpointBoot(cfg.dir, explicitFlags, &selectedRelease, &selectedSumsSHA256, &runtimeRelease, &runtimeSums); err != nil {
		fatal(uiTextWith("fatal.linux.could_not_prepare_the_snapshot_you_rolled_back", map[string]string{"error": fmt.Sprintf("%v", err)}))
	} else if pinned {
		logf("snapshots: first boot after roll back uses its saved system files (%s)", releaseVersion(selectedRelease))
	}
	stopGuestUpdate := func() {}
	if explicitFlags["release"] || explicitFlags["sums-sha256"] {
		err = ensureLinuxGuest(cfg, selectedRelease, selectedSumsSHA256)
	} else {
		stopGuestUpdate, err = configureLinuxGuestBootFirst(cfg, selectedRelease, selectedSumsSHA256)
	}
	if err != nil {
		failLinuxSetup(err, cfg.dir)
	}
	defer stopGuestUpdate()
	specData, err := os.ReadFile(filepath.Join(cfg.guestDir, "build-spec.json"))
	if err != nil {
		fatal(uiTextWith("fatal.build_spec.read", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	var spec buildSpec
	if err := json.Unmarshal(specData, &spec); err != nil {
		fatal(uiTextWith("fatal.build_spec.parse", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}
	cfg.guestPinch = guestAcceptsPinch(spec)
	cfg.followHostTimeZone = strings.TrimSpace(*timeZoneFlag) != "keep" && guestAcceptsTimeZone(spec)
	// Tells the guest which host it runs on, for wording such as approved apps.
	cmdline := windowedKernelCmdline(spec) + " tryomarchy.host=linux"
	if linuxGuestScale != "keep" {
		cmdline += " tryomarchy.host-scale=1"
	}
	if cfg.instant {
		cmdline += " tryomarchy.instant=1"
	}
	cmdline += sshCmdline(cfg.forwards, cfg.sshKey)
	cmdline += shareCmdline(cfg.share)
	if words := hostLocaleCmdline(hostLocale(*timeZoneFlag, *keyboardFlag, *localeFlag)); words != "" {
		cmdline += words
		logf("guest follows this computer's locale:%s", words)
	}

	if cfg.useGpu {
		present, err := vulkanPresentMode(*vulkanPresentFlag, onlyNVIDIARenderNodes(renderNodeVendors("/sys/class/drm")))
		if err != nil {
			fatalf("%v", err)
		}
		if present == "cpu" {
			cmdline += " tryomarchy.vulkan-present=cpu"
			logf("guest Vulkan windows present through CPU copies (%s)", *vulkanPresentFlag)
		}
	}

	if err := prepareDisk(cfg, spec.Runtime.Storage.ExpandedSizeMiB); err != nil {
		failLinuxSetup(err, cfg.dir)
	}

	profile := effectiveResourceProfile(resourcePrefs.Profile, cfg.cpuOverride, cfg.memOverrideMiB)
	host := measureHostResources(profile == resourceMaximum)
	allocation, err := planGuestResources(profile, host, cfg.useGpu, cfg.cpuOverride, cfg.memOverrideMiB,
		explicitFlags["cpus"], explicitFlags["memory"])
	if err != nil {
		fatal(uiTextWith("fatal.resources", map[string]string{"error": linuxResourceErrorText(err)}))
	}
	cfg.cpus, cfg.memMiB, cfg.hostTotalMiB = allocation.CPUs, allocation.MemoryMiB, host.TotalMiB
	logf("resources: profile=%s, %d of %d logical processors, %d MiB guest RAM; host available=%d MiB, CPU sample known=%t busy=%.1f%%",
		profile, cfg.cpus, host.LogicalCPUs, cfg.memMiB, host.AvailableMiB, host.CPUKnown, host.CPUBusy*100)

	cfg.displayWidth, cfg.displayHeight = *width, *height
	cmdline += fmt.Sprintf(" video=%dx%d", cfg.displayWidth, cfg.displayHeight)
	if err := checkSetupCancelled(); err != nil {
		fatalf("%v", err)
	}

	// Optional sharing comes after downloads and disk preparation succeed.
	// A failed or cancelled install must not request access to the clipboard.
	// Keep the setup window alive to explain GNOME's permission before boot.
	stopClipboard := runLinuxClipboardBridge()
	defer stopClipboard()
	if err := checkSetupCancelled(); err != nil {
		fatalf("%v", err)
	}

	configureLinuxReclaim(cfg)
	go runLinuxGuestAgent(cfg.dir)
	runCameraBridge(cfg.desktop)
	if cfg.followHostTimeZone {
		override := strings.TrimSpace(*timeZoneFlag)
		stopTimeZone, err := startLinuxTimeZoneBridge(func() string {
			if override != "" {
				return override
			}
			return liveHostTimeZone()
		})
		if err != nil {
			logf("live time-zone following unavailable: %v", err)
			cfg.followHostTimeZone = false
		} else {
			defer stopTimeZone()
			logf("guest follows this computer's time zone while it runs")
		}
	}
	if err := checkForwardBindings(cfg.forwards); err != nil {
		fatal(uiTextWith("fatal.forwarding", map[string]string{"error": fmt.Sprintf("%v", err)}))
	}

	// The first interrupt asks the guest to shut down; a second one stops QEMU.
	stop := make(chan os.Signal, 2)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	if err := superviseLinux(cfg, cmdline, stop); err != nil {
		reportLinuxQEMUFailure(err)
		os.Exit(1)
	}
	compactLinuxDisk(cfg)
	logf("---- exiting ----")
}

// validateLinuxSharedFolder accepts an existing directory outside the data
// folder, whose disk image the guest must not reach through the share.
func validateLinuxSharedFolder(path, dataDir string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	dataDir, err = resolveLinuxFutureDirectory(dataDir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", uiError(uiText("share.linux.not_folder"), nil)
	}
	if linuxPathWithin(abs, dataDir) {
		return "", uiError(uiText("share.linux.contains_data"), nil)
	}
	if linuxPathWithin(dataDir, abs) {
		return "", uiError(uiText("share.linux.inside_data"), nil)
	}
	return abs, nil
}

// Resolve each existing ancestor, including aliases, while allowing the
// destination itself to be absent on the first Settings visit.
func resolveLinuxFutureDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	for ancestor := abs; ; ancestor = filepath.Dir(ancestor) {
		if _, err := os.Lstat(ancestor); err == nil {
			resolved, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return "", err
			}
			info, err := os.Stat(resolved)
			if err != nil {
				return "", err
			}
			if !info.IsDir() {
				return "", fmt.Errorf("data location has a non-folder ancestor: %s", ancestor)
			}
			remainder, err := filepath.Rel(ancestor, abs)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, remainder), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if ancestor == filepath.Dir(ancestor) {
			return "", fmt.Errorf("cannot resolve data location %s", path)
		}
	}
}

func linuxPathWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

// runLinuxGuestAgent serves the guest agent: clock sync, reclaim, status and
// the apps on this computer that Settings approved for launching from Omarchy.
func runLinuxGuestAgent(dir string) {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", agentPort))
	if err != nil {
		logf("agent: port %d unavailable, guest clock sync disabled: %v", agentPort, err)
		return
	}
	logf("agent: listening on %d", agentPort)
	a := newGuestAgent()
	a.peerAllowed = lifecycleConnectionFromQEMU
	a.health = &guestCompositorHealth
	a.appsDir = dir
	a.appsMinVersion = linuxHostAppsAgentVersion
	a.launchApp = func(id string) error {
		err := launchApprovedLinuxApp(dir, id)
		if err != nil {
			logf("apps: %v", err)
		}
		return err
	}
	a.reclaimFinished = linuxReclaimFinished
	a.dropDrag = performLinuxDropDrag
	theAgent.Store(a)
	a.run(l, hostResumed)
}

// vulkanPresentMode resolves -vulkan-present. Automatic copies frames on
// hosts where only NVIDIA can import them, since its GL misreads the pitch.
func vulkanPresentMode(flagValue string, nvidiaOnly bool) (string, error) {
	switch flagValue {
	case "auto":
		if nvidiaOnly {
			return "cpu", nil
		}
		return "gpu", nil
	case "gpu", "cpu":
		return flagValue, nil
	}
	return "", uiError(uiText("error.linux.vulkan_present"), nil)
}
