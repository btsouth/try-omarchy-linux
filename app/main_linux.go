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
	"time"
)

// Try Omarchy for Linux. It prepares the guest the same way the Windows
// launcher does and runs it with KVM in QEMU's SDL window. A separate GTK
// process shows setup progress; the launcher itself remains cgo-free.

// failLinuxSetup reports a setup failure in plain words with a Try again that
// restarts setup from what is on disk: downloads continue where they stopped.
func failLinuxSetup(err error, dir string) {
	if setupCancelled() || errors.Is(err, errSetupCancelled) {
		fatal("%v", err)
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

func fatal(format string, a ...any) {
	if setupCancelled() {
		getUI().finish()
		logf("Setup cancelled; existing disks and downloaded files retained")
		os.Exit(0)
	}
	msg := fmt.Sprintf(format, a...)
	logf("FATAL %s", msg)
	fmt.Fprintf(os.Stderr, "%s: %s\n", appTitle, msg)
	getUI().showError(msg)
	os.Exit(1)
}

// Inside the Flatpak XDG_DATA_HOME is the app's own data folder.
func defaultLinuxDataDirectory() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "try-omarchy")
}

func main() {
	cfg := &config{}
	flag.StringVar(&cfg.dir, "dir", defaultLinuxDataDirectory(), "Try Omarchy data directory (virtual machine and settings)")
	flag.StringVar(&cfg.qemu, "qemu", "qemu-system-x86_64", "QEMU to run")
	flag.StringVar(&cfg.share, "share", "", "folder shared into Omarchy at /mnt/host and as ~/<folder name>")
	chooseShare := flag.Bool("choose-share", false, "choose or remove the shared folder before starting")
	flag.BoolVar(&cfg.fresh, "fresh", false, "start over and retain the previous writable disk for recovery")
	flag.BoolVar(&cfg.fullscreen, "fullscreen", false, "start fullscreen")
	flag.IntVar(&cfg.memOverrideMiB, "memory", 0, "guest RAM in MiB (default: sized to this computer)")
	flag.IntVar(&cfg.cpuOverride, "cpus", 0, "guest CPUs (default: sized to this computer)")
	resourceProfileFlag := flag.String("resource-profile", "", "resource preset: balanced, maximum-performance, or manual")
	flag.IntVar(&cfg.diskGiB, "disk-size", 0, "guest disk capacity in GiB (0: default; grows existing disks, never shrinks)")
	renderFlag := flag.String("render", "", "rendering path: auto (default), gpu, or cpu")
	venusFlag := flag.String("venus", "auto", "Vulkan acceleration: auto (compatible KVM kernels), on, or off; off keeps hardware OpenGL")
	scaleFlag := flag.String("scale", "auto", "guest UI scale: auto follows Wayland, keep uses the guest policy, or 1 through 4")
	audioFlag := flag.String("audio", "auto", "audio backend: auto, pipewire, sdl, or none")
	audioOutput := flag.String("audio-output", "", "PipeWire output node name; blank follows the default")
	audioInput := flag.String("audio-input", "", "PipeWire input node name; blank follows the default")
	microphone := flag.Bool("microphone", true, "allow the guest to use a microphone")
	vulkanPresentFlag := flag.String("vulkan-present", "auto", "how guest Vulkan windows reach the screen: auto, gpu, or cpu (cpu copies frames and avoids the NVIDIA import bug)")
	timeZoneFlag := flag.String("timezone", "", "guest time zone: blank follows this computer, keep leaves the guest alone, or an IANA name")
	keyboardFlag := flag.String("keyboard", "", "guest keyboard layout: blank follows exposed host XKB settings, keep preserves the guest, or an XKB layout such as de or us:intl")
	localeFlag := flag.String("locale", "", "guest language: blank follows this computer, keep leaves the guest alone, or a locale such as de_DE")
	flag.BoolVar(&cfg.instant, "instant", false, "skip the account question and use the quick-start omarchy account")
	var forwards forwardList
	flag.Var(&forwards, "forward", "forward a local port into Omarchy: tcp:2222:22; repeatable")
	sshPort := flag.Int("ssh", 0, "forward this loopback port to Omarchy's sshd and start sshd for the session")
	sshKeyPath := flag.String("ssh-key", "", "public key to authorize for the Omarchy account (default: your ~/.ssh/id_*.pub when -ssh is used)")
	width := flag.Int("width", 1280, "initial guest display width")
	height := flag.Int("height", 800, "initial guest display height")
	release := flag.String("release", linuxGuestReleaseURL, "base URL the guest image is downloaded from on first run")
	sumsSHA256 := flag.String("sums-sha256", linuxGuestSumsSHA256, "trusted SHA256 digest of the release's SHA256SUMS file")
	noGUI := flag.Bool("no-gui", false, "show setup status in the terminal only")
	startDirect := flag.Bool("start", false, "start Omarchy without the launcher home")
	showLauncher := flag.Bool("launcher", false, "show the launcher home even with other options")
	flag.Parse()
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
		fatal("Could not finish moving the Omarchy data folder: %v", err)
	}
	if explicitFlags["dir"] {
		resolved, err := resolveLinuxMovedDirectory(defaultLinuxDataDirectory(), cfg.dir)
		if err != nil {
			fatal("Cannot read the data folder move record: %v", err)
		}
		cfg.dir = resolved
	}
	if !*noGUI && !*startDirect && (*showLauncher || !linuxDirectStart(explicitFlags)) {
		if !showLinuxHome(defaultLinuxDataDirectory(), cfg.dir, explicitFlags["dir"]) {
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
		fatal("Cannot select the Linux Omarchy image: %v", err)
	}

	if err := checkKVM(); err != nil {
		fatal("%v.", err)
	}
	qemu, err := exec.LookPath(cfg.qemu)
	if err != nil {
		fatal("Cannot find QEMU (%s): %v", cfg.qemu, err)
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
		fatal("Cannot select the data folder: %v", err)
	}
	if !proceed {
		return
	}
	cfg.dir = selected
	if !explicitFlags["dir"] && !pathsEqual(selected, defaultLinuxDataDirectory()) {
		noteLinuxLocationHint(defaultLinuxDataDirectory(), selected)
	}
	if cfg.dir, err = filepath.Abs(cfg.dir); err != nil {
		fatal("Cannot resolve the data directory: %v", err)
	}
	cfg.hostDir = cfg.dir
	cfg.guestDir = filepath.Join(cfg.dir, "guest")
	cfg.vmDir = filepath.Join(cfg.dir, "vm")
	cfg.diskFormat = "raw"
	cfg.disk = filepath.Join(cfg.vmDir, "disk.raw")
	if err := os.MkdirAll(cfg.vmDir, 0o755); err != nil {
		fatal("Could not create the Omarchy data directory: %v", err)
	}
	if shellLog, _ := os.OpenFile(filepath.Join(cfg.vmDir, "shell.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); shellLog != nil {
		openLog(shellLog)
	}
	logf("---- %s starting (Linux) ----", appTitle)
	if err := configureLinuxGraphics(*venusFlag); err != nil {
		fatal("%v", err)
	}
	if cfg.audio, err = linuxAudioMode(*audioFlag); err != nil {
		fatal("%v", err)
	}

	if cfg.desktop, err = loadDesktopPreferences(cfg.dir); err != nil {
		fatal("Cannot read device and update preferences: %v", err)
	}
	if cfg.audioDevices, err = loadAudioPreferences(cfg.dir); err != nil {
		fatal("Cannot read audio preferences: %v", err)
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
		fatal("Cannot use audio devices: %v", err)
	}
	experience, err := loadLinuxExperiencePreferences(cfg.dir)
	if err != nil {
		fatal("Cannot read display and keyboard preferences: %v", err)
	}
	if !explicitFlags["scale"] {
		*scaleFlag = experience.Scale
	}
	if !explicitFlags["keyboard"] {
		*keyboardFlag = experience.Keyboard
	}
	if linuxGuestScale, err = parseLinuxScale(*scaleFlag); err != nil {
		fatal("%v", err)
	}
	userSettings, err := loadSettings(settingsPath(cfg.dir))
	if err != nil {
		fatal("Cannot read its settings: %v\n\nFix or delete %s and try again.", err, settingsPath(cfg.dir))
	}
	if err := applySettings(cfg, userSettings, explicitFlags, &forwards, sshKeyPath); err != nil {
		fatal("Cannot use its settings: %v", err)
	}
	resourcePrefs, err := loadResourcePreferences(cfg.dir)
	if err != nil {
		fatal("Cannot read resource preferences: %v", err)
	}
	if explicitFlags["resource-profile"] {
		resourcePrefs.Profile = *resourceProfileFlag
	}
	if err := validateResourceProfile(resourcePrefs.Profile); err != nil {
		fatal("%v", err)
	}
	if explicitFlags["render"] {
		if cfg.renderMode, err = parseRenderMode(*renderFlag); err != nil {
			fatal("%v", err)
		}
	}
	if cfg.memOverrideMiB != 0 && (cfg.memOverrideMiB < minimumGuestMemoryMiB || cfg.memOverrideMiB > maximumGuestMemoryMiB) {
		fatal("-memory must be between %d and %d MiB.", minimumGuestMemoryMiB, maximumGuestMemoryMiB)
	}
	if !explicitFlags["disk-size"] {
		storage, err := loadStorageSettings(cfg.dir)
		if err != nil {
			fatal("Cannot read storage preferences: %v", err)
		}
		cfg.diskGiB = storage.DiskGiB
	}
	if _, err := requestedDiskMiB(24*1024, cfg.diskGiB, false); err != nil {
		fatal("%v", err)
	}
	home, _ := os.UserHomeDir()
	if cfg.sshKey, err = resolveSSHPreset(&forwards, *sshPort, *sshKeyPath, home, explicitFlags["ssh-key"]); err != nil {
		fatal("%v.", err)
	}
	cfg.forwards = forwards
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
		fatal("Cannot select the account setup: %v", err)
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
		fatal("Choosing a shared folder requires the setup window. Use -share with an accessible folder in terminal mode.")
	}
	if err := configureLinuxSharing(cfg, &userSettings, explicitFlags["share"], *chooseShare, chooseFolder); err != nil {
		fatal("Cannot configure the shared folder: %v", err)
	}
	if cfg.share != "" {
		if cfg.share, err = validateLinuxSharedFolder(cfg.share, cfg.dir); err != nil {
			fatal("Cannot share %s: %v", cfg.share, err)
		}
	}
	if err := checkSetupCancelled(); err != nil {
		fatal("%v", err)
	}

	var reason string
	cfg.useGpu, reason = startWithGPU(cfg.renderMode, nil, "", "", time.Now())
	if reason != "" {
		logf("rendering: %s", reason)
	}

	if err := recoverLinuxGuestUpdate(cfg.dir, &selectedRelease, &selectedSumsSHA256); err != nil {
		fatal("Could not restore the previous Omarchy image after an interrupted update: %v", err)
	}
	if err := ensureLinuxGuest(cfg, selectedRelease, selectedSumsSHA256); err != nil {
		failLinuxSetup(err, cfg.dir)
	}
	specData, err := os.ReadFile(filepath.Join(cfg.guestDir, "build-spec.json"))
	if err != nil {
		fatal("Cannot read build-spec.json: %v", err)
	}
	var spec buildSpec
	if err := json.Unmarshal(specData, &spec); err != nil {
		fatal("Cannot parse build-spec.json: %v", err)
	}
	cfg.guestPinch = guestAcceptsPinch(spec)
	cmdline := windowedKernelCmdline(spec)
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
			fatal("%v", err)
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
		fatal("Cannot allocate resources: %v", err)
	}
	cfg.cpus, cfg.memMiB, cfg.hostTotalMiB = allocation.CPUs, allocation.MemoryMiB, host.TotalMiB
	logf("resources: profile=%s, %d of %d logical processors, %d MiB guest RAM; host available=%d MiB, CPU sample known=%t busy=%.1f%%",
		profile, cfg.cpus, host.LogicalCPUs, cfg.memMiB, host.AvailableMiB, host.CPUKnown, host.CPUBusy*100)

	cfg.displayWidth, cfg.displayHeight = *width, *height
	cmdline += fmt.Sprintf(" video=%dx%d", cfg.displayWidth, cfg.displayHeight)
	if err := checkSetupCancelled(); err != nil {
		fatal("%v", err)
	}

	// Optional sharing comes after downloads and disk preparation succeed.
	// A failed or cancelled install must not request access to the clipboard.
	// Keep the setup window alive to explain GNOME's permission before boot.
	stopClipboard := runLinuxClipboardBridge()
	defer stopClipboard()
	if err := checkSetupCancelled(); err != nil {
		fatal("%v", err)
	}

	reclaimDir.Store(&cfg.dir)
	reclaimSupported.Store(true)
	go runLinuxGuestAgent(cfg.dir)
	runCameraBridge(cfg.desktop)
	if err := checkForwardBindings(cfg.forwards); err != nil {
		fatal("Could not prepare port forwarding:\n\n%v", err)
	}

	// The first interrupt asks the guest to shut down; a second one stops QEMU.
	stop := make(chan os.Signal, 2)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	superviseLinux(cfg, cmdline, stop)
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
		return "", fmt.Errorf("not a folder")
	}
	if linuxPathWithin(abs, dataDir) {
		return "", fmt.Errorf("it contains the Try Omarchy data folder")
	}
	if linuxPathWithin(dataDir, abs) {
		return "", fmt.Errorf("it is inside the Try Omarchy data folder")
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

// runLinuxGuestAgent serves the guest agent: clock sync, reclaim and status.
// Approved host apps are a Windows feature, so launchApp stays unset.
func runLinuxGuestAgent(dir string) {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", agentPort))
	if err != nil {
		logf("agent: port %d unavailable, guest clock sync disabled: %v", agentPort, err)
		return
	}
	logf("agent: listening on %d", agentPort)
	a := newGuestAgent()
	a.appsDir = dir
	theAgent.Store(a)
	a.run(l, make(chan struct{}))
}

// compactLinuxDisk returns zero-filled blocks to the host after a reclaim.
func compactLinuxDisk(cfg *config) {
	a := theAgent.Load()
	if a == nil || !a.compactPending() {
		return
	}
	getUI().setStatus("Reclaiming disk space...")
	before, _ := platformAllocatedFileBytes(cfg.disk)
	reclaimed, err := compactDisk(cfg.disk, nil)
	if err != nil {
		getUI().setStatus("Disk space could not be reclaimed: %v", err)
		return
	}
	after, _ := platformAllocatedFileBytes(cfg.disk)
	getUI().setStatus("Reclaimed %s (%s of zero blocks)", formatGiB(max(before-after, 0)), formatGiB(reclaimed))
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
	return "", fmt.Errorf("-vulkan-present must be auto, gpu, or cpu")
}
