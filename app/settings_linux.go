//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type linuxSettingsForm struct {
	Memory             string             `json:"memory"`
	CPUs               string             `json:"cpus"`
	Render             string             `json:"render"`
	Fullscreen         bool               `json:"fullscreen"`
	Microphone         bool               `json:"microphone"`
	Camera             bool               `json:"camera"`
	CameraID           string             `json:"cameraID"`
	Cameras            []linuxAudioDevice `json:"cameras,omitempty"`
	AudioOutput        string             `json:"audioOutput"`
	AudioInput         string             `json:"audioInput"`
	AudioOutputs       []linuxAudioDevice `json:"audioOutputs,omitempty"`
	AudioInputs        []linuxAudioDevice `json:"audioInputs,omitempty"`
	AudioLive          bool               `json:"audioLive"`
	RefreshAudio       bool               `json:"refreshAudio,omitempty"`
	DiskGiB            string             `json:"diskGiB"`
	Scale              string             `json:"scale"`
	Keyboard           string             `json:"keyboard"`
	SSHEnabled         bool               `json:"sshEnabled"`
	SSHPort            string             `json:"sshPort"`
	SSHKey             string             `json:"sshKey"`
	Forwards           string             `json:"forwards"`
	StartAutomatically bool               `json:"startAutomatically"`
	Share              string             `json:"share"`
	ShareEnabled       bool               `json:"shareEnabled"`
	ResourceProfile    string             `json:"resourceProfile"`
	ResourceSummary    string             `json:"resourceSummary"`
	CPUMax             int                `json:"cpuMax"`
	// Only shown on GNOME Wayland, where clipboard sharing needs a permission.
	ClipboardShare     bool `json:"clipboardShare"`
	ClipboardAvailable bool `json:"clipboardAvailable"`
}

var linuxSettingsOpen atomic.Bool
var linuxRecoveryActive atomic.Bool

func showLinuxSettings(parent context.Context, dir string) {
	if !linuxSettingsOpen.CompareAndSwap(false, true) {
		return
	}
	defer linuxSettingsOpen.Store(false)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	w := startLinuxWindow(cancel)
	if w == nil {
		return
	}
	defer w.stop()
	showLinuxSettingsInWindow(ctx, w, dir, true)
}

// The home and running VM use the same form. PipeWire device routes can
// change live; microphone permission and the other host settings need a launch.
func showLinuxSettingsInWindow(ctx context.Context, w *linuxSetupWindow, dir string, running bool) string {
	saved, err := loadSettings(settingsPath(dir))
	if err != nil {
		logf("settings: %v", err)
		return "Could not read settings: " + err.Error()
	}
	desktop, err := loadDesktopPreferences(dir)
	if err != nil {
		logf("desktop preferences: %v", err)
		return "Could not read device preferences: " + err.Error()
	}
	audio, err := loadAudioPreferences(dir)
	if err != nil {
		return "Could not read audio preferences: " + err.Error()
	}
	storage, err := loadStorageSettings(dir)
	if err != nil {
		return "Could not read storage preferences: " + err.Error()
	}
	experience, err := loadLinuxExperiencePreferences(dir)
	if err != nil {
		return "Could not read display and keyboard preferences: " + err.Error()
	}
	launch, err := loadLaunchPreferences(dir)
	if err != nil {
		return "Could not read startup preferences: " + err.Error()
	}
	resources, err := loadResourcePreferences(dir)
	if err != nil {
		return "Could not read resource preferences: " + err.Error()
	}
	outputs, inputs, audioListErr := listLinuxAudioDevices()
	cameras, cameraListErr := listLinuxCameraDevices()
	sshEnabled, sshPort, additionalForwards := linuxNetworkForm(saved.Forwards)
	form := &linuxSettingsForm{Memory: strconv.Itoa(saved.MemoryMiB), CPUs: strconv.Itoa(saved.CPUs), Render: saved.Render, Fullscreen: saved.Fullscreen, Microphone: !desktop.MicrophoneDisabled, Camera: !desktop.CameraDisabled, CameraID: desktop.CameraID, Cameras: cameras, AudioOutput: audio.Output, AudioInput: audio.Input, AudioOutputs: outputs, AudioInputs: inputs, DiskGiB: strconv.Itoa(storage.DiskGiB), Scale: experience.Scale, Keyboard: experience.Keyboard, SSHEnabled: sshEnabled, SSHPort: sshPort, SSHKey: saved.SSHKey, Forwards: additionalForwards, StartAutomatically: launch.StartAutomatically, Share: saved.Share, ShareEnabled: saved.Share != "" && !saved.ShareDisabled, CPUMax: min(maximumGuestCPUs, max(1, measureHostResources(false).LogicalCPUs)), ClipboardAvailable: linuxGNOMEWayland(), ClipboardShare: !linuxClipboardSharingOff()}
	form.ResourceProfile = effectiveResourceProfile(resources.Profile, saved.CPUs, saved.MemoryMiB)
	form.ResourceSummary = linuxAutomaticResourcesSummary(measureHostResources(false))
	form.AudioLive = running && linuxLiveAudioAvailable(ctx)
	clipboardShared := form.ClipboardShare
	if form.Render == "" {
		form.Render = "auto"
	}
	status := "Changes apply when the VM starts."
	if running {
		status = "Changes require shutting down Omarchy and launching it again. A guest reboot does not apply host settings."
		if form.AudioLive {
			status = "Audio device choices apply when you save. Microphone access, camera settings and other changes need a shutdown and launch."
		}
	}
	if audioListErr != nil {
		status += " Audio devices could not be listed. System default remains available."
	}
	if cameraListErr != nil {
		status += " Camera devices could not be listed. Automatic selection remains available."
	}
	if running {
		status += "\n\nCamera: " + cameraStatusText()
		if clipboardStatus, ok := linuxClipboardStatus.Load().(string); ok && clipboardStatus != "" {
			status += "\n\nClipboard: " + clipboardStatus
		}
	}
	for {
		value, err := w.ask(ctx, linuxSetupState{Prompt: "settings", Status: status, Settings: form})
		if err != nil || value == "cancel" {
			return ""
		}
		audioLive := form.AudioLive
		resourceSummary := form.ResourceSummary
		audioOutputs, audioInputs := form.AudioOutputs, form.AudioInputs
		cameras := form.Cameras
		err = json.Unmarshal([]byte(value), form)
		form.AudioLive = audioLive
		form.ResourceSummary = resourceSummary
		form.AudioOutputs, form.AudioInputs = audioOutputs, audioInputs
		form.Cameras = cameras
		if err != nil {
			status = "Could not read the settings."
			continue
		}
		if form.RefreshAudio {
			form.RefreshAudio = false
			form.AudioOutputs, form.AudioInputs, err = listLinuxAudioDevices()
			var cameraErr error
			form.Cameras, cameraErr = listLinuxCameraDevices()
			status = "Devices refreshed. Your changes have not been saved."
			if err != nil || cameraErr != nil {
				status = "Some devices could not be listed. Your changes have not been saved."
			}
			continue
		}
		forwardsText, err := linuxForwardsFromForm(form.SSHEnabled, form.SSHPort, form.Forwards)
		var next settings
		if err == nil && form.SSHKey != "" && (form.SSHKey != saved.SSHKey || form.SSHEnabled && !sshEnabled) {
			_, err = loadPublicKey(form.SSHKey)
		}
		if err == nil {
			next, err = settingsFromForm(form.Fullscreen, form.ShareEnabled && form.Share != "", form.Memory, form.CPUs, form.Share, forwardsText, "", form.Render)
			next.SSHKey = form.SSHKey
		}
		if err == nil {
			err = validateLinuxLocalForwards(next.Forwards)
		}
		if err == nil && !running && strings.Join(next.Forwards, "\n") != strings.Join(saved.Forwards, "\n") {
			var forwards forwardList
			for _, value := range next.Forwards {
				if err = forwards.Set(value); err != nil {
					break
				}
			}
			if err == nil {
				err = checkForwardBindings(forwards)
			}
		}
		cpuMax := min(maximumGuestCPUs, max(1, measureHostResources(false).LogicalCPUs))
		if err == nil && next.CPUs > cpuMax {
			err = fmt.Errorf("choose at most %d processors on this computer", cpuMax)
		}
		if err == nil && form.Share != "" && form.Share != saved.Share {
			next.Share, err = validateLinuxSharedFolder(form.Share, dir)
		}
		if err == nil && (len(form.CameraID) > 4096 || strings.ContainsRune(form.CameraID, 0)) {
			err = fmt.Errorf("invalid camera selection")
		}
		if err == nil {
			err = validateResourceProfile(form.ResourceProfile)
		}
		nextAudio := audio
		nextAudio.Output, nextAudio.Input = form.AudioOutput, form.AudioInput
		if err == nil {
			err = nextAudio.validate()
		}
		nextDiskGiB := storage.DiskGiB
		if err == nil {
			nextDiskGiB, err = parseDiskGiB(form.DiskGiB)
		}
		if err == nil && nextDiskGiB != 0 {
			if info, statErr := os.Stat(filepath.Join(dir, "vm", "disk.raw")); statErr == nil {
				if int64(nextDiskGiB)*1024*1024*1024 < info.Size() {
					err = fmt.Errorf("the existing disk is already %d GiB; choose at least that capacity", (info.Size()+1024*1024*1024-1)/(1024*1024*1024))
				}
			} else if !os.IsNotExist(statErr) {
				err = statErr
			}
		}
		nextExperience := experience
		nextExperience.Scale, nextExperience.Keyboard = form.Scale, form.Keyboard
		if err == nil {
			err = nextExperience.validate()
		}
		if err == nil {
			// Preserve fields absent from this Linux form.
			saved.MemoryMiB, saved.CPUs, saved.Render, saved.Fullscreen = next.MemoryMiB, next.CPUs, next.Render, next.Fullscreen
			if form.Share != saved.Share || form.ShareEnabled != (saved.Share != "" && !saved.ShareDisabled) {
				saved.Share, saved.ShareDisabled, saved.SharedFolderPrompted = next.Share, next.ShareDisabled, true
			}
			saved.Forwards, saved.SSHKey = next.Forwards, next.SSHKey
			err = saveSettings(settingsPath(dir), saved)
			if err == nil && form.ResourceProfile != resources.Profile {
				err = saveResourcePreferences(dir, form.ResourceProfile)
				if err == nil {
					resources.Profile = form.ResourceProfile
				}
			}
			if err == nil && (desktop.MicrophoneDisabled == form.Microphone || desktop.CameraDisabled == form.Camera || desktop.CameraID != form.CameraID) {
				nextDesktop := desktop
				nextDesktop.MicrophoneDisabled = !form.Microphone
				nextDesktop.CameraDisabled = !form.Camera
				nextDesktop.CameraID = form.CameraID
				err = saveDesktopPreferences(dir, nextDesktop)
				if err == nil {
					desktop = nextDesktop
				}
			}
			if err == nil && (nextAudio.Output != audio.Output || nextAudio.Input != audio.Input) {
				err = saveAudioPreferences(dir, nextAudio)
				if err == nil {
					audio = nextAudio
				}
			}
			if err == nil && nextDiskGiB != storage.DiskGiB {
				err = saveStorageSettings(dir, nextDiskGiB)
				if err == nil {
					storage.DiskGiB = nextDiskGiB
				}
			}
			if err == nil && (nextExperience.Scale != experience.Scale || nextExperience.Keyboard != experience.Keyboard) {
				err = saveLinuxExperiencePreferences(dir, nextExperience)
				if err == nil {
					experience = nextExperience
				}
			}
			if err == nil && form.ClipboardAvailable && form.ClipboardShare != clipboardShared {
				err = setLinuxClipboardSharing(form.ClipboardShare)
				if err == nil {
					clipboardShared = form.ClipboardShare
				}
			}
			if err == nil && form.StartAutomatically != launch.StartAutomatically {
				nextLaunch := launch
				nextLaunch.StartAutomatically = form.StartAutomatically
				err = saveLaunchPreferences(dir, nextLaunch)
				if err == nil {
					launch = nextLaunch
				}
			}
		}
		if err == nil && form.AudioLive {
			// Always retry a live apply, even if an earlier Save already persisted
			// these names but QMP failed. A failed disk write never changes routes.
			if routeErr := applyLinuxAudioRoutes(ctx, audio); routeErr != nil {
				err = fmt.Errorf("Audio choices saved, but could not switch devices: %v. Try Save again, or shut down Omarchy and launch it again", routeErr)
			}
		}
		if err == nil {
			if running {
				message := "Settings saved. Shut down Omarchy and launch it again to apply them. A guest reboot does not restart the VM."
				if form.AudioLive {
					message = "Audio device choices applied. Microphone access, camera settings and other settings apply after shutting down Omarchy and launching it again."
				}
				w.ask(ctx, linuxSetupState{Prompt: "settings-saved", Status: message})
				return message
			}
			return "Settings saved. They will apply when you launch Omarchy."
		}
		status = err.Error()
	}
}

// The shared sizing policy predates the Linux launcher; adapt only its host
// wording here, without changing Windows diagnostics or allocation behavior.
func linuxResourceErrorText(err error) string {
	return strings.NewReplacer("Windows headroom", "Linux headroom", "Windows apps", "Linux apps", "memory for Windows", "memory for Linux").Replace(err.Error())
}

// This is an estimate, not a reservation or the running VM's allocation.
// Automatic graphics selection can change memory sizing at launch.
func linuxAutomaticResourcesSummary(host hostResources) string {
	cpu := pickGuestCPUs(host.LogicalCPUs)
	low := pickGuestMemMiB(false, host.TotalMiB, host.AvailableMiB)
	high := pickGuestMemMiB(true, host.TotalMiB, host.AvailableMiB)
	memory := fmt.Sprintf("%.1f GiB", float64(low)/1024)
	if high != low {
		memory = fmt.Sprintf("%.1f–%.1f GiB", float64(min(low, high))/1024, float64(max(low, high))/1024)
	}
	return fmt.Sprintf("Automatic estimate now: %d processors, %s memory depending on rendering. Checked again at launch. Choose Manual to override memory or processors.", cpu, memory)
}

// A desktop launch has no flags. Explicit CLI options keep their historical
// direct-start behavior, while -launcher lets a script request the home.
func linuxDirectStart(flags map[string]bool) bool {
	for name := range flags {
		if name != "launcher" && name != "start" && name != "no-gui" {
			return true
		}
	}
	return false
}

// linuxKVMCheck is a variable so the home's notice can be tested.
var linuxKVMCheck = checkKVM

func linuxHomeState(defaultDir string) (linuxSetupState, string) {
	dir := defaultDir
	if saved, found, err := loadDataLocationPointer(defaultDir); err != nil {
		logf("home: the saved location record cannot be read: %v", err)
		return linuxSetupState{Prompt: "home", Path: defaultDir, Version: linuxAppVersion, CanForget: true,
			Headline: "Try Omarchy cannot read where your VM is saved.",
			Status:   "Choose your VM's folder again, or forget the saved location to start fresh. Nothing is deleted either way."}, ""
	} else if found {
		dir = saved
		if _, err := os.Stat(dir); err != nil {
			logf("home: the saved VM folder is unavailable: %v", err)
			return linuxSetupState{Prompt: "home", Path: dir, Version: linuxAppVersion, CanForget: true,
				Headline: "Your Omarchy folder is not available.",
				Status: "Try Omarchy remembers your VM at " + linuxLocationHostPath(defaultDir, dir) + ", but cannot open it now. " +
					"If it is on a drive, reconnect it. If you reinstalled the app, choose the folder again. Nothing has been deleted."}, ""
		}
	}
	return linuxHomeStateForDir(dir, defaultDir)
}

func linuxHomeStateForDir(dir, defaultDir string) (linuxSetupState, string) {
	state := linuxSetupState{Prompt: "home", Path: dir, Version: linuxAppVersion}
	free := linuxFreeBytes(dir)
	if _, err := os.Stat(filepath.Join(dir, "vm", "disk.raw")); err == nil {
		state.Installed = true
		state.Headline = "Omarchy is ready."
		state.Status = "Your files are saved in this VM. Launch to open your desktop."
		state.Sections = []linuxSection{{Heading: "Storage", Rows: linuxStorageRows(dir, defaultDir, false)}}
	} else if os.IsNotExist(err) {
		state.Headline = "Omarchy is not set up yet."
		state.Status = "Set up Omarchy asks how you want to sign in, then downloads Omarchy and starts it. You can move or delete it later."
		state.Detail = "About " + linuxGB(linuxGuestDownloadBytes) + " to download and " + linuxGB(linuxGuestSpaceBytes) + " of free space needed.\nCustomize lets you choose another folder first."
		state.Sections = []linuxSection{{Heading: "What setup does", Rows: linuxSetupRows(dir, defaultDir, free)}}
	} else {
		state.Headline = "Omarchy's storage cannot be read."
		state.Status = "Cannot read virtual machine storage: " + err.Error()
	}
	state.Sections = append(state.Sections, linuxSection{Heading: "Integrations", Rows: linuxHomeIntegrationRows(dir)})
	if err := linuxKVMCheck(); err != nil {
		state.Notice, state.HelpURL, state.CheckAgain = "KVM is not available.", linuxHelpURL("kvm"), true
		var kvm *kvmError
		if errors.As(err, &kvm) {
			state.Notice = kvm.Short
		}
		state.Headline, state.Status = "Omarchy cannot start yet.", capitalizeFirst(err.Error())+"."
	} else if free >= 0 && state.Installed && free < linuxLowSpaceBytes {
		state.Notice, state.HelpURL = "Only "+linuxGB(free)+" is free on this drive.", linuxHelpURL("space")
		state.Status = "Omarchy can stop working if the drive fills up. Free some space, then launch."
	} else if free >= 0 && !state.Installed && free < linuxGuestSpaceBytes {
		state.Notice, state.HelpURL = "Only "+linuxGB(free)+" is free here.", linuxHelpURL("space")
		state.Status = "Setup needs about " + linuxGB(linuxGuestSpaceBytes) + ". Free some space, or choose another folder."
		state.Setup = "customize"
	}
	if state.Notice == "" && state.Installed {
		if notice := linuxLeftoverNotice(defaultDir); notice != "" {
			state.Notice = notice
		}
	}
	return state, dir
}

// Saved choices only: building this card never opens devices or asks portals
// for permission. Running integration status belongs to the running Settings.
func linuxHomeIntegrationRows(dir string) []linuxRow {
	prefs, err := loadSettings(settingsPath(dir))
	if err != nil {
		return []linuxRow{{Title: "Settings", Detail: "Saved choices could not be read. Open Settings to see the problem.", State: "unavailable"}}
	}
	resources, resourceErr := loadResourcePreferences(dir)
	desktop, deviceErr := loadDesktopPreferences(dir)
	if resourceErr != nil || deviceErr != nil {
		return []linuxRow{{Title: "Settings", Detail: "Saved device or resource choices could not be read. Open Settings to see the problem.", State: "unavailable"}}
	}
	profile := effectiveResourceProfile(resources.Profile, prefs.CPUs, prefs.MemoryMiB)
	profileName := map[string]string{resourceBalanced: "Balanced", resourceMaximum: "Maximum performance", resourceManual: "Manual"}[profile]
	rows := []linuxRow{{Title: "Resources", Detail: profileName + " · checked at the next launch", State: "pending"}}
	share := linuxRow{Title: "Shared folder", Detail: "Not shared", State: "disabled"}
	if prefs.Share != "" && !prefs.ShareDisabled {
		share.Detail, share.State = "Shared at the next launch: "+prefs.Share, "enabled"
	}
	rows = append(rows, share)
	for _, device := range []struct {
		title    string
		disabled bool
	}{{"Microphone", desktop.MicrophoneDisabled}, {"Camera", desktop.CameraDisabled}} {
		r := linuxRow{Title: device.title, Detail: "Access allowed at the next launch", State: "enabled"}
		if device.disabled {
			r.Detail, r.State = "Access disabled at the next launch", "disabled"
		}
		if device.title == "Camera" && !device.disabled {
			r.Detail += "; your desktop asks for permission when needed"
		}
		rows = append(rows, r)
	}
	return rows
}

// linuxSetupRows is what a first setup will do, in the order it matters:
// what is downloaded, what it costs in space, where it goes and who you are.
func linuxSetupRows(dir, defaultDir string, free int64) []linuxRow {
	space := "About " + linuxGB(linuxGuestSpaceBytes) + ". Omarchy sees a " + linuxGB(int64(24)<<30) + " disk, but only what it uses takes space."
	if free >= 0 {
		space += " " + linuxGB(free) + " is free here."
	}
	location := linuxStorageRows(dir, defaultDir, false)[0]
	return []linuxRow{
		{Title: "Download", Detail: "About " + linuxGB(linuxGuestDownloadBytes) + ", once. If it is interrupted, it continues where it stopped."},
		{Title: "Space", Detail: space},
		location,
		{Title: "Account", Detail: "Your own username and password, or a quick-start account signed in as " + trialUsername + " (password " + trialPassword + "). You choose before Omarchy starts."},
	}
}

// linuxHomeActions decides the home's buttons for its state. Storage that
// cannot be opened offers the way back to it first; a computer without KVM
// offers to check again; a first setup offers one button that just works.
func linuxHomeActions(state linuxSetupState) (actions, menu []linuxAction) {
	more := []linuxAction{{Label: "About and help", Reply: "about"}}
	if state.CanAttach {
		more = append(more, linuxAction{Label: "Use existing data folder", Reply: "attach"})
	}
	if state.CanDelete {
		more = append(more, linuxAction{Label: "Delete this VM...", Reply: "delete-default", Destructive: true})
	}
	switch {
	case state.CanForget:
		actions = []linuxAction{{Label: "Use existing data folder", Reply: "attach", Suggested: true}, {Label: "Forget saved location", Reply: "forget"}, {Label: "About and help", Reply: "about"}, {Label: "Close", Reply: "close"}}
		return actions, nil
	case state.CheckAgain:
		actions = append(actions, linuxAction{Label: "Check again", Reply: "check", Suggested: true})
	case !state.Installed && state.Setup == "customize":
		actions = append(actions, linuxAction{Label: "Choose another folder...", Reply: "customize", Suggested: true})
	case !state.Installed:
		actions = append(actions, linuxAction{Label: "Set up Omarchy", Reply: "setup", Suggested: true}, linuxAction{Label: "Customize...", Reply: "customize"})
	default:
		actions = append(actions, linuxAction{Label: "Launch Omarchy", Reply: "launch", Suggested: true})
	}
	actions = append(actions, linuxAction{Label: "Settings", Reply: "settings"})
	if state.Installed {
		actions = append(actions, linuxAction{Label: "Backup and recovery", Reply: "recovery"})
	}
	actions = append(actions, linuxAction{Label: "Close", Reply: "close"})
	return actions, more
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func showLinuxHome(defaultDir, requestedDir string, explicitDir bool) bool {
	w := startLinuxWindow(func() {
		if linuxRecoveryActive.Load() {
			requestSetupCancel()
		}
	})
	if w == nil {
		// A missing helper keeps the terminal/flag workflow available.
		return true
	}
	defer w.stop()
	return runLinuxHome(w, defaultDir, requestedDir, explicitDir)
}

// runLinuxHome shows the home until the person launches or closes it, and
// reports whether to launch.
func runLinuxHome(w *linuxSetupWindow, defaultDir, requestedDir string, explicitDir bool) bool {
	status := ""
	for {
		state, dir := linuxHomeState(defaultDir)
		if explicitDir {
			resolved, err := resolveLinuxMovedDirectory(defaultDir, requestedDir)
			if err != nil {
				state = linuxSetupState{Prompt: "home", Status: "Cannot read the data folder move record: " + err.Error()}
				dir = ""
			} else {
				state, dir = linuxHomeStateForDir(resolved, defaultDir)
			}
		}
		state.CanAttach = !explicitDir
		state.CanDelete = !explicitDir && !state.CanForget && dir != "" && pathsEqual(dir, defaultDir) && linuxDefaultVMCanDelete(defaultDir)
		if status != "" {
			state.Status = status
		}
		ctx := context.Background()
		cancel := func() {}
		if dir != "" {
			launch, err := loadLaunchPreferences(dir)
			if err != nil {
				state.Status = "Could not read startup preferences: " + err.Error()
			} else if launch.StartAutomatically && status == "" {
				state.Status = "Omarchy starts automatically in 10 seconds. Choose Settings or Close to stop."
				ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
			}
		}
		// The window draws exactly the buttons decided here.
		state.Actions, state.Menu = linuxHomeActions(state)
		answer, err := w.ask(ctx, state)
		cancel()
		if errors.Is(err, context.DeadlineExceeded) {
			return true
		}
		if err != nil || answer == "close" {
			return false
		}
		switch answer {
		case "check":
			status = ""
		case "launch", "customize", "setup":
			// Check again after a fix that needs no restart, such as loading the
			// KVM module. The banner keeps saying what is wrong until it is gone.
			state.Sections = append(state.Sections, linuxSection{Heading: "Integrations", Rows: linuxHomeIntegrationRows(dir)})
			if err := linuxKVMCheck(); err != nil {
				status = ""
				continue
			}
			linuxQuickSetup.Store(answer == "setup")
			return true
		case "settings":
			if dir == "" {
				status = "Reconnect or repair the saved storage location before changing settings."
				continue
			}
			status = showLinuxSettingsInWindow(context.Background(), w, dir, false)
		case "recovery":
			status = showLinuxRecoveryInWindow(w, defaultDir, dir)
		case "attach":
			selected, err := w.ask(context.Background(), linuxSetupState{Prompt: "attach-folder", Status: "Choose an existing Try Omarchy data folder containing your VM. This only changes which VM Try Omarchy opens. No files are moved or deleted."})
			if err != nil || selected == "cancel" {
				status = ""
				continue
			}
			selected, err = attachLinuxDataDirectory(defaultDir, selected)
			if err != nil {
				status = "Could not use that data folder: " + err.Error()
			} else {
				status = "Using the existing VM at " + selected + ". No files were moved or deleted."
			}
		case "forget":
			prompt := "Forget the saved location at " + state.Path + "?"
			if pathsEqual(state.Path, defaultDir) {
				prompt = "Forget the unreadable saved location record in " + defaultDir + "?"
			}
			answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "forget-location", Path: state.Path, Status: prompt + " This only removes Try Omarchy's location choice. The VM files and shared folders stay where they are. You can reattach the VM later."})
			if err != nil || answer != "forget" {
				status = ""
				continue
			}
			if err := forgetLinuxDataLocation(defaultDir); err != nil {
				status = "Could not forget the saved location: " + err.Error()
			} else {
				status = "Saved location forgotten. Its files were not deleted. You can reconnect the drive or choose an existing data folder later."
			}
		case "delete-default":
			answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "delete-default", Path: defaultDir, Status: "Permanently delete this VM and its downloaded guest files from " + defaultDir + "? Files inside Omarchy will be lost. Shared host folders and backups outside this location will stay. You can create a new VM later."})
			if err != nil || answer != "delete" {
				status = "The VM was kept."
				continue
			}
			if err := deleteLinuxDefaultVM(defaultDir); err != nil {
				status = "Could not delete the VM: " + err.Error()
			} else {
				status = "The VM and downloaded guest files were deleted from " + defaultDir + ". Shared host folders and backups were kept."
			}
		case "about":
			_, err := w.ask(context.Background(), linuxAboutState())
			if err != nil {
				return false
			}
		}
	}
}
