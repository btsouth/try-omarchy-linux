package main

// Launch configuration shared by the platform front ends. Each front end fills
// it from its flags, settings and host probes before setup and launch.

const appTitle = "Try Omarchy"

type config struct {
	desktop                     desktopPreferences
	audioDevices                audioPreferences
	audioRates                  audioSampleRates
	dir, hostDir, payloadDir    string
	winqEmu, share              string
	fresh, fullscreen, noGpu    bool
	fullscreenDisplay           string
	hostCursor                  bool
	experimentalPinch           bool
	disablePinch, guestPinch    bool
	followHostTimeZone          bool
	lanPublic                   bool
	instant, portable           bool
	guestDir, vmDir, disk       string
	qmpDir                      string
	diskFormat                  string
	qemu                        string
	useGpu                      bool
	supportsSharing             bool
	audio                       string
	memMiB                      int
	displays                    int
	displayWidth, displayHeight int
	// Linux per-output placement and initial EDID sizes.
	displayTargets    []string
	displayFullscreen []bool
	displaySizes      [][2]int
	// kernel-irqchip=off keeps WHPX from requesting nested virtualization,
	// which some hosts advertise and then refuse (issue #19). Set by the
	// startup retry, never by a flag.
	forwards []portForward
	// launchForwards is the list from this launch; forwards follows live
	// changes from Settings between boots (forward_live.go).
	launchForwards []portForward
	sshKey         string
	// Guest RAM chosen by the user (settings.json or -memory); 0 = automatic.
	memOverrideMiB int
	diskGiB        int
	irqchipOff     bool
	// Guest vCPUs chosen by the user (settings.json or -cpus); 0 = automatic.
	cpuOverride  int
	cpus         int
	hostTotalMiB int
	// Rendering decision inputs, see render_probe.go.
	renderMode    string
	runtimeID     string
	displayDriver string
}
