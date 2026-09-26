//go:build linux

package main

import (
	"encoding/json"
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
// launcher does and runs it with KVM in QEMU's SDL window. There is no setup
// window yet: status goes to the terminal and to shell.log in the vm folder.

func fatal(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	logf("FATAL %s", msg)
	fmt.Fprintf(os.Stderr, "%s: %s\n", appTitle, msg)
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
	flag.BoolVar(&cfg.fresh, "fresh", false, "start over and retain the previous writable disk for recovery")
	flag.BoolVar(&cfg.fullscreen, "fullscreen", false, "start fullscreen")
	flag.IntVar(&cfg.memOverrideMiB, "memory", 0, "guest RAM in MiB (default: sized to this computer)")
	flag.IntVar(&cfg.cpuOverride, "cpus", 0, "guest CPUs (default: sized to this computer)")
	resourceProfileFlag := flag.String("resource-profile", "", "resource preset: balanced, maximum-performance, or manual")
	flag.IntVar(&cfg.diskGiB, "disk-size", 0, "guest disk capacity in GiB (0: default; grows existing disks, never shrinks)")
	renderFlag := flag.String("render", "", "rendering path: auto (default), gpu, or cpu")
	timeZoneFlag := flag.String("timezone", "", "guest time zone: blank follows this computer, keep leaves the guest alone, or an IANA name")
	keyboardFlag := flag.String("keyboard", "", "guest keyboard layout: blank or keep leaves the guest alone, or an XKB layout such as de or us:intl")
	localeFlag := flag.String("locale", "", "guest language: blank follows this computer, keep leaves the guest alone, or a locale such as de_DE")
	flag.BoolVar(&cfg.instant, "instant", false, "skip first-boot questions and use the trial account")
	var forwards forwardList
	flag.Var(&forwards, "forward", "forward a local port into Omarchy: tcp:2222:22; repeatable")
	sshPort := flag.Int("ssh", 0, "forward this loopback port to Omarchy's sshd and start sshd for the session")
	sshKeyPath := flag.String("ssh-key", "", "public key to authorize for the Omarchy account (default: your ~/.ssh/id_*.pub when -ssh is used)")
	width := flag.Int("width", 1280, "initial guest display width")
	height := flag.Int("height", 800, "initial guest display height")
	release := flag.String("release", defaultReleaseURL, "base URL the guest image is downloaded from on first run")
	sumsSHA256 := flag.String("sums-sha256", defaultSumsSHA256, "trusted SHA256 digest of the release's SHA256SUMS file")
	flag.Parse()
	explicitFlags := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicitFlags[f.Name] = true })

	if err := checkKVM(); err != nil {
		fatal("%v.", err)
	}
	qemu, err := exec.LookPath(cfg.qemu)
	if err != nil {
		fatal("Cannot find QEMU (%s): %v", cfg.qemu, err)
	}
	cfg.qemu = qemu
	cfg.supportsSharing = true
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

	if cfg.desktop, err = loadDesktopPreferences(cfg.dir); err != nil {
		fatal("Cannot read device and update preferences: %v", err)
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
	if cfg.share != "" {
		if cfg.share, err = validateLinuxSharedFolder(cfg.share, cfg.dir); err != nil {
			fatal("Cannot share %s: %v", cfg.share, err)
		}
	}

	var reason string
	cfg.useGpu, reason = startWithGPU(cfg.renderMode, nil, "", "", time.Now())
	if reason != "" {
		logf("rendering: %s", reason)
	}

	if err := ensureGuest(cfg, *release, *sumsSHA256); err != nil {
		fatal("Setting up the Omarchy image failed: %v", err)
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
	if cfg.instant {
		cmdline += " tryomarchy.instant=1"
	}
	cmdline += sshCmdline(cfg.forwards, cfg.sshKey)
	cmdline += shareCmdline(cfg.share)
	if words := hostLocaleCmdline(hostLocale(*timeZoneFlag, *keyboardFlag, *localeFlag)); words != "" {
		cmdline += words
		logf("guest follows this computer's locale:%s", words)
	}

	if err := prepareDisk(cfg, spec.Runtime.Storage.ExpandedSizeMiB); err != nil {
		fatal("Preparing the writable disk failed: %v", err)
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

	runLifecycleListener()
	reclaimDir.Store(&cfg.dir)
	reclaimSupported.Store(true)
	go runLinuxGuestAgent(cfg.dir)
	runCameraBridge(cfg.desktop)
	if err := checkForwardBindings(cfg.forwards); err != nil {
		fatal("Could not prepare port forwarding:\n\n%v", err)
	}
	cfg.audio = "pipewire"

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
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a folder")
	}
	if rel, err := filepath.Rel(abs, dataDir); err == nil && !strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("it contains the Try Omarchy data folder")
	}
	if rel, err := filepath.Rel(dataDir, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("it is inside the Try Omarchy data folder")
	}
	return abs, nil
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
