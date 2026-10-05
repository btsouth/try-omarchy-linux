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

// linuxSettingsForm is one Settings snapshot. Reclaim is nil before launch;
// StartReclaim asks for a reclaim pass without saving the form.
type linuxSettingsForm struct {
	Memory             string             `json:"memory"`
	CPUs               string             `json:"cpus"`
	Render             string             `json:"render"`
	Fullscreen         bool               `json:"fullscreen"`
	FullscreenDisplay  string             `json:"fullscreenDisplay"`
	Microphone         bool               `json:"microphone"`
	Camera             bool               `json:"camera"`
	CameraID           string             `json:"cameraID"`
	Cameras            []linuxAudioDevice `json:"cameras,omitempty"`
	AudioOutput        string             `json:"audioOutput"`
	AudioInput         string             `json:"audioInput"`
	AudioOutputs       []linuxAudioDevice `json:"audioOutputs,omitempty"`
	AudioInputs        []linuxAudioDevice `json:"audioInputs,omitempty"`
	AudioLive          bool               `json:"audioLive"`
	ForwardsLive       bool               `json:"forwardsLive"`
	Running            bool               `json:"running"`
	RefreshAudio       bool               `json:"refreshAudio,omitempty"`
	Reclaim            *linuxReclaimInfo  `json:"reclaim,omitempty"`
	StartReclaim       bool               `json:"startReclaim,omitempty"`
	DiskGiB            string             `json:"diskGiB"`
	Scale              string             `json:"scale"`
	Keyboard           string             `json:"keyboard"`
	HostKeyboard       string             `json:"hostKeyboard"`
	SSHEnabled         bool               `json:"sshEnabled"`
	SSHPort            string             `json:"sshPort"`
	SSHKey             string             `json:"sshKey"`
	Forwards           string             `json:"forwards"`
	StartAutomatically bool               `json:"startAutomatically"`
	LaunchAtSignIn     bool               `json:"launchAtSignIn"`
	Share              string             `json:"share"`
	ShareDisplay       string             `json:"shareDisplay,omitempty"`
	ShareEnabled       bool               `json:"shareEnabled"`
	ResourceProfile    string             `json:"resourceProfile"`
	ResourceSummary    string             `json:"resourceSummary"`
	CPUMax             int                `json:"cpuMax"`
	// Shown for both Wayland and X11 clipboard sharing.
	ClipboardShare     bool `json:"clipboardShare"`
	ClipboardAvailable bool `json:"clipboardAvailable"`
	// HostApps is the approved list, by desktop file ID, and the only part the
	// window sends back. HostAppChoices is what this computer has installed.
	HostApps       []linuxHostApp `json:"hostApps"`
	HostAppChoices []linuxHostApp `json:"hostAppChoices,omitempty"`
	HostAppsNote   string         `json:"hostAppsNote,omitempty"`
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

// The home and running VM use the same form. PipeWire device routes and local
// port forwards can change live; microphone permission and the other host
// settings need a launch.
func showLinuxSettingsInWindow(ctx context.Context, w *linuxSetupWindow, dir string, running bool) string {
	saved, err := loadSettings(settingsPath(dir))
	if err != nil {
		logf("settings: %v", err)
		return uiTextWith("settings.linux.could_not_read_settings", map[string]string{"error": err.Error()})
	}
	desktop, err := loadDesktopPreferences(dir)
	if err != nil {
		logf("desktop preferences: %v", err)
		return uiTextWith("settings.linux.could_not_read_device_preferences", map[string]string{"error": err.Error()})
	}
	audio, err := loadAudioPreferences(dir)
	if err != nil {
		return uiTextWith("settings.linux.could_not_read_audio_preferences", map[string]string{"error": err.Error()})
	}
	storage, err := loadStorageSettings(dir)
	if err != nil {
		return uiTextWith("settings.linux.could_not_read_storage_preferences", map[string]string{"error": err.Error()})
	}
	experience, err := loadLinuxExperiencePreferences(dir)
	if err != nil {
		return uiTextWith("settings.linux.could_not_read_display_and_keyboard_preferences", map[string]string{"error": err.Error()})
	}
	launch, err := loadLaunchPreferences(dir)
	if err != nil {
		return uiTextWith("settings.linux.could_not_read_startup_preferences", map[string]string{"error": err.Error()})
	}
	resources, err := loadResourcePreferences(dir)
	if err != nil {
		return uiTextWith("settings.linux.could_not_read_resource_preferences", map[string]string{"error": err.Error()})
	}
	approvedApps, err := loadApprovedWindowsApps(dir)
	if err != nil {
		return uiTextWith("settings.linux.could_not_read_approved_apps", map[string]string{"error": err.Error()})
	}
	appsCtx, appsDone := context.WithTimeout(ctx, 10*time.Second)
	hostFiles, hostAppsErr := readHostDesktopFiles(appsCtx)
	appsDone()
	hostApps := linuxHostApps(hostFiles)
	outputs, inputs, audioListErr := listLinuxAudioDevices()
	cameras, cameraListErr := listLinuxCameraDevices()
	sshEnabled, sshPort, additionalForwards := linuxNetworkForm(saved.Forwards)
	form := &linuxSettingsForm{Memory: strconv.Itoa(saved.MemoryMiB), CPUs: strconv.Itoa(saved.CPUs), Render: saved.Render, Fullscreen: saved.Fullscreen, FullscreenDisplay: saved.FullscreenDisplay, Microphone: !desktop.MicrophoneDisabled, Camera: !desktop.CameraDisabled, CameraID: desktop.CameraID, Cameras: cameras, AudioOutput: audio.Output, AudioInput: audio.Input, AudioOutputs: outputs, AudioInputs: inputs, DiskGiB: strconv.Itoa(storage.DiskGiB), Scale: experience.Scale, Keyboard: experience.Keyboard, SSHEnabled: sshEnabled, SSHPort: sshPort, SSHKey: saved.SSHKey, Forwards: additionalForwards, StartAutomatically: launch.StartAutomatically, LaunchAtSignIn: launch.LaunchAtSignIn, Share: saved.Share, ShareEnabled: saved.Share != "" && !saved.ShareDisabled, CPUMax: min(maximumGuestCPUs, max(1, measureHostResources(false).LogicalCPUs)), ClipboardAvailable: os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") != "", ClipboardShare: !linuxClipboardSharingOff()}
	layout, variant := linuxHostKeyboard()
	form.HostKeyboard = layout
	if variant != "" {
		form.HostKeyboard += ":" + variant
	}
	form.ResourceProfile = effectiveResourceProfile(resources.Profile, saved.CPUs, saved.MemoryMiB)
	form.ResourceSummary = linuxAutomaticResourcesSummary(measureHostResources(false))
	form.AudioLive = running && linuxLiveAudioAvailable(ctx)
	form.ForwardsLive = running && linuxLiveForwards.Load()
	form.Running = running
	form.HostApps, form.HostAppChoices = approvedLinuxHostApps(approvedApps), hostApps
	if hostAppsErr != nil {
		logf("apps: %v", hostAppsErr)
		form.HostAppsNote = uiText("settings.linux.the_apps_on_this_computer_could_not_be")
	}
	if running {
		form.Reclaim = linuxReclaimInfoFor(dir)
	}
	clipboardShared := form.ClipboardShare
	if form.Render == "" {
		form.Render = "auto"
	}
	status := linuxSettingsTiming(running, form.AudioLive, form.ForwardsLive)
	if audioListErr != nil {
		status += uiText("settings.linux.audio_devices_could_not_be_listed_system_default")
	}
	if cameraListErr != nil {
		status += uiText("settings.linux.camera_devices_could_not_be_listed_automatic_selection")
	}
	if running {
		status += "\n\nCamera: " + cameraStatusText()
		if clipboardStatus, ok := linuxClipboardStatus.Load().(string); ok && clipboardStatus != "" {
			status += "\n\nClipboard: " + clipboardStatus
		}
	}
	notice := ""
	for {
		form.ShareDisplay = linuxSharedFolderDisplayPath(form.Share)
		value, err := w.ask(ctx, linuxSetupState{Prompt: "settings", Status: status, Notice: notice, Settings: form})
		if err != nil || value == "cancel" {
			return ""
		}
		audioLive, forwardsLive := form.AudioLive, form.ForwardsLive
		resourceSummary := form.ResourceSummary
		audioOutputs, audioInputs := form.AudioOutputs, form.AudioInputs
		cameras := form.Cameras
		form.StartReclaim = false
		form.HostApps = nil
		err = json.Unmarshal([]byte(value), form)
		form.HostAppChoices = hostApps
		chosenApps := make([]string, 0, len(form.HostApps))
		for _, app := range form.HostApps {
			chosenApps = append(chosenApps, app.ID)
		}
		nextApps, appsErr := approveLinuxHostApps(approvedApps, chosenApps, hostApps)
		if appsErr == nil {
			// Refreshing devices keeps the unsaved choices with their real names.
			form.HostApps = approvedLinuxHostApps(nextApps)
		} else {
			form.HostApps = approvedLinuxHostApps(approvedApps)
		}
		form.AudioLive, form.ForwardsLive = audioLive, forwardsLive
		form.Running = running
		form.ResourceSummary = resourceSummary
		form.AudioOutputs, form.AudioInputs = audioOutputs, audioInputs
		form.Cameras = cameras
		form.Reclaim = nil
		if running {
			form.Reclaim = linuxReclaimInfoFor(dir)
		}
		if err != nil {
			status = uiText("settings.linux.could_not_read_the_settings")
			notice = status
			continue
		}
		if form.StartReclaim {
			form.StartReclaim = false
			notice = ""
			// The Storage card shows the result. Unsaved edits stay in the form.
			if running {
				form.Reclaim = startLinuxReclaimFromSettings(dir)
			}
			continue
		}
		if form.RefreshAudio {
			notice = ""
			form.RefreshAudio = false
			form.AudioOutputs, form.AudioInputs, err = listLinuxAudioDevices()
			var cameraErr error
			form.Cameras, cameraErr = listLinuxCameraDevices()
			status = uiText("settings.linux.devices_refreshed_your_changes_have_not_been_saved")
			if err != nil || cameraErr != nil {
				status = uiText("settings.linux.some_devices_could_not_be_listed_your_changes")
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
			next.FullscreenDisplay = strings.TrimSpace(form.FullscreenDisplay)
			if err == nil && (len(next.FullscreenDisplay) > 64 || strings.ContainsAny(next.FullscreenDisplay, ",\x00\r\n")) {
				err = uiError(uiText("settings.linux.fullscreen_display_name_is_invalid"), nil)
			}
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
		if err == nil && form.ForwardsLive {
			// The running VM holds its current ports, so only a forward on a
			// port it does not have yet needs that port free.
			var plan forwardPlan
			if plan, err = planLinuxLiveForwards(next.Forwards); err == nil {
				err = checkForwardBindings(newForwardPorts(plan))
			}
		}
		cpuMax := min(maximumGuestCPUs, max(1, measureHostResources(false).LogicalCPUs))
		if err == nil && next.CPUs > cpuMax {
			err = uiError(uiTextWith("settings.linux.choose_at_most_processors_on_this_computer", map[string]string{"cpu_max": fmt.Sprintf("%d", cpuMax)}), nil)
		}
		if err == nil && form.Share != "" && form.Share != saved.Share {
			next.Share, err = validateLinuxSharedFolder(form.Share, dir)
		}
		if err == nil && (len(form.CameraID) > 4096 || strings.ContainsRune(form.CameraID, 0)) {
			err = uiError(uiText("settings.linux.invalid_camera_selection"), nil)
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
					err = uiError(uiTextWith("settings.linux.the_existing_disk_is_already_gib_choose_at", map[string]string{"value": fmt.Sprintf("%d", (info.Size()+1024*1024*1024-1)/(1024*1024*1024))}), nil)
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
			err = appsErr
		}
		var savedGroups []string
		savePart := func(group string, write func() error) error {
			if writeErr := write(); writeErr != nil {
				return &linuxSettingsSaveError{Group: group, Saved: append([]string(nil), savedGroups...), Err: writeErr}
			}
			savedGroups = append(savedGroups, group)
			return nil
		}
		if err == nil {
			// Preserve fields absent from this Linux form. Update our last-saved
			// copy only after the atomic write succeeds.
			nextSaved := saved
			nextSaved.MemoryMiB, nextSaved.CPUs, nextSaved.Render, nextSaved.Fullscreen = next.MemoryMiB, next.CPUs, next.Render, next.Fullscreen
			nextSaved.FullscreenDisplay = next.FullscreenDisplay
			if form.Share != saved.Share || form.ShareEnabled != (saved.Share != "" && !saved.ShareDisabled) {
				nextSaved.Share, nextSaved.ShareDisabled, nextSaved.SharedFolderPrompted = next.Share, next.ShareDisabled, true
			}
			nextSaved.Forwards, nextSaved.SSHKey = next.Forwards, next.SSHKey
			err = savePart(uiText("settings.linux.vm_configuration"), func() error { return saveSettings(settingsPath(dir), nextSaved) })
			if err == nil {
				saved = nextSaved
			}
			if err == nil && form.ResourceProfile != resources.Profile {
				err = savePart(uiText("settings.linux.resource_profile"), func() error { return saveResourcePreferences(dir, form.ResourceProfile) })
				if err == nil {
					resources.Profile = form.ResourceProfile
				}
			}
			if err == nil && (desktop.MicrophoneDisabled == form.Microphone || desktop.CameraDisabled == form.Camera || desktop.CameraID != form.CameraID) {
				nextDesktop := desktop
				nextDesktop.MicrophoneDisabled = !form.Microphone
				nextDesktop.CameraDisabled = !form.Camera
				nextDesktop.CameraID = form.CameraID
				err = savePart(uiText("settings.linux.camera_and_microphone_access"), func() error { return saveDesktopPreferences(dir, nextDesktop) })
				if err == nil {
					desktop = nextDesktop
				}
			}
			if err == nil && (nextAudio.Output != audio.Output || nextAudio.Input != audio.Input) {
				err = savePart(uiText("settings.linux.audio_devices"), func() error { return saveAudioPreferences(dir, nextAudio) })
				if err == nil {
					audio = nextAudio
				}
			}
			if err == nil && nextDiskGiB != storage.DiskGiB {
				err = savePart(uiText("settings.linux.disk_capacity"), func() error { return saveStorageSettings(dir, nextDiskGiB) })
				if err == nil {
					storage.DiskGiB = nextDiskGiB
				}
			}
			if err == nil && (nextExperience.Scale != experience.Scale || nextExperience.Keyboard != experience.Keyboard) {
				err = savePart(uiText("settings.linux.display_and_keyboard_2"), func() error { return saveLinuxExperiencePreferences(dir, nextExperience) })
				if err == nil {
					experience = nextExperience
				}
			}
			if err == nil && form.ClipboardAvailable && form.ClipboardShare != clipboardShared {
				err = savePart(uiText("settings.linux.clipboard_sharing"), func() error { return setLinuxClipboardSharing(form.ClipboardShare) })
				if err == nil {
					clipboardShared = form.ClipboardShare
				}
			}
			if err == nil && !sameApprovedApps(nextApps, approvedApps) {
				err = savePart(uiText("settings.linux.approved_apps"), func() error { return saveApprovedWindowsApps(dir, nextApps) })
				if err == nil {
					approvedApps = nextApps
					if a := theAgent.Load(); a != nil {
						a.sendApprovedApps()
					}
				}
			}
			if err == nil && (form.StartAutomatically != launch.StartAutomatically || form.LaunchAtSignIn != launch.LaunchAtSignIn) {
				nextLaunch := launch
				nextLaunch.StartAutomatically = form.StartAutomatically
				nextLaunch.LaunchAtSignIn = form.LaunchAtSignIn
				err = savePart("startup", func() error { return saveLinuxLaunchPreferences(ctx, dir, launch, nextLaunch) })
				if err == nil {
					launch = nextLaunch
				}
			}
		}
		liveApplyFailed, forwardApplyFailed := false, false
		var forwardChange linuxForwardChange
		if err == nil && form.AudioLive {
			// Always retry a live apply, even if an earlier Save already persisted
			// these names but QMP failed. A failed disk write never changes routes.
			if routeErr := applyLinuxAudioRoutes(ctx, audio); routeErr != nil {
				liveApplyFailed = true
				err = uiError(uiTextWith("settings.linux.audio_choices_saved_but_could_not_switch_devices", map[string]string{"error": fmt.Sprintf("%v", routeErr)}), nil)
			}
		}
		if err == nil && form.ForwardsLive {
			var forwardErr error
			if forwardChange, forwardErr = applyLinuxLiveForwards(ctx, saved.Forwards); forwardErr != nil {
				forwardApplyFailed = true
				err = uiError(uiTextWith("settings.linux.port_forwards_saved_but_the_running_vm_could", map[string]string{"error": fmt.Sprintf("%v", forwardErr)}), nil)
			}
		}
		if err == nil {
			if running {
				message := linuxSettingsSavedMessage(form.AudioLive, forwardChange)
				w.ask(ctx, linuxSetupState{Prompt: "settings-saved", Status: message})
				return message
			}
			return uiTextWith("settings.linux.settings_saved", map[string]string{"value": linuxSettingsTiming(false, false, false)})
		}
		status = err.Error()
		notice = uiText("settings.linux.check_your_settings_before_saving")
		if liveApplyFailed {
			notice = uiText("settings.linux.audio_choices_saved_live_switch_failed")
		}
		if forwardApplyFailed {
			notice = linuxForwardFailureNotice(forwardChange)
		}
		var partial *linuxSettingsSaveError
		if errors.As(err, &partial) {
			notice = uiTextWith("settings.linux.could_not_save", map[string]string{"group": partial.Group})
			logf("settings save (%s): %v", partial.Group, partial.Err)
			if form.AudioLive {
				status += uiText("settings.linux.audio_devices_have_not_been_switched_in_the")
			}
		} else {
			logf("settings save: %v", err)
		}
	}
}

// Settings live in independent, atomically replaced files. Earlier writes
// remain saved if a later group fails; the form stays open with every edit.
type linuxSettingsSaveError struct {
	Group string
	Saved []string
	Err   error
}

func (e *linuxSettingsSaveError) Error() string {
	message := uiTextWith("settings.linux.could_not_save", map[string]string{"group": e.Group})
	var loginErr linuxSignInError
	portalFailure := errors.As(e.Err, &loginErr)
	if portalFailure {
		message = loginErr.Error()
	}
	if len(e.Saved) > 0 {
		message += uiTextWith("settings.linux.already_saved", map[string]string{"names": strings.Join(e.Saved, ", ")})
	} else {
		message += uiText("settings.linux.no_settings_were_saved_by_this_attempt")
	}
	if portalFailure {
		return uiTextWith("settings.linux.your_remaining_edits_are_kept_here", map[string]string{"message": message})
	}
	folder := uiText("settings.linux.vm_folder")
	if e.Group == uiText("settings.linux.clipboard_sharing") {
		folder = uiText("settings.linux.app_s_configuration_folder")
	}
	return uiTextWith("settings.linux.your_remaining_edits_are_kept_here_check_that", map[string]string{"message": message, "folder": folder})
}

func (e *linuxSettingsSaveError) Unwrap() error { return e.Err }

func linuxSettingsTiming(running, audioLive, forwardsLive bool) string {
	message := uiText("settings.linux.vm_settings_apply_on_the_next_launch_startup")
	if running {
		message = uiText("settings.linux.vm_settings_apply_after_shutting_down_omarchy_and")
	}
	switch {
	case audioLive && forwardsLive:
		message = uiTextWith("settings.linux.audio_device_choices_and_local_port_forwards_apply", map[string]string{"message": message})
	case audioLive:
		message = uiTextWith("settings.linux.audio_device_choices_apply_when_you_save", map[string]string{"message": message})
	case forwardsLive:
		message = uiTextWith("settings.linux.local_port_forwards_apply_when_you_save", map[string]string{"message": message})
	}
	return message
}

// linuxForwardFailureNotice keeps partial live updates visible when a later
// monitor command fails. Successfully applied forwards stay active for retry.
func linuxForwardFailureNotice(change linuxForwardChange) string {
	if change.changed {
		return uiText("settings.linux.port_forwards_saved_some_applied_but_the_live")
	}
	return uiText("settings.linux.port_forwards_saved_live_update_failed")
}

// linuxSettingsSavedMessage says what a save in the running VM changed now
// and what waits for the next launch.
func linuxSettingsSavedMessage(audioLive bool, forwards linuxForwardChange) string {
	var applied []string
	if audioLive {
		applied = append(applied, uiText("settings.linux.audio_device_choices_applied"))
	}
	if forwards.changed {
		applied = append(applied, uiText("settings.linux.port_forwards_applied"))
	}
	message := uiText("settings.linux.settings_saved_2")
	if len(applied) > 0 {
		message = strings.Join(applied, " ")
	}
	if len(forwards.deferred) > 0 {
		message += uiText("settings.linux.ssh_changes_apply_after_shutting_down_omarchy_and")
	}
	return message + " " + linuxSettingsTiming(true, false, false)
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
	return uiTextWith("settings.linux.current_estimate_processors_ram_depends_on_rendering_checked", map[string]string{"cpu": fmt.Sprintf("%d", cpu), "memory": memory})
}

// A desktop launch has no flags. Explicit CLI options keep their historical
// direct-start behavior, while -launcher lets a script request the home.
func linuxDirectStart(flags map[string]bool) bool {
	for name := range flags {
		if name != "launcher" && name != "start" && name != "no-gui" && name != "autostart" {
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
			Headline: uiText("settings.linux.try_omarchy_cannot_read_where_your_vm_is"),
			Status:   uiText("settings.linux.choose_your_vm_s_folder_again_or_forget")}, ""
	} else if found {
		dir = saved
		if _, err := os.Stat(dir); err != nil {
			logf("home: the saved VM folder is unavailable: %v", err)
			return linuxSetupState{Prompt: "home", Path: dir, Version: linuxAppVersion, CanForget: true,
				Headline: uiText("settings.linux.your_omarchy_folder_is_not_available"),
				Status:   uiTextWith("settings.linux.try_omarchy_remembers_your_vm_at_but_cannot", map[string]string{"path": linuxLocationHostPath(defaultDir, dir)})}, ""
		}
	}
	return linuxHomeStateForDir(dir, defaultDir)
}

// linuxHomeStateForDir reads storage, saved integrations and host readiness for
// the selected installation without creating files or activating devices.
func linuxHomeStateForDir(dir, defaultDir string) (linuxSetupState, string) {
	state := linuxSetupState{Prompt: "home", Path: dir, Version: linuxAppVersion}
	free := linuxFreeBytes(dir)
	if _, err := os.Stat(filepath.Join(dir, "vm", "disk.raw")); err == nil {
		state.Installed = true
		state.Headline = uiText("settings.linux.omarchy_is_ready")
		state.Status = uiText("settings.linux.your_files_are_saved_in_this_vm_launch")
		state.Sections = []linuxSection{{Heading: uiText("settings.linux.storage"), Rows: linuxStorageRows(dir, defaultDir, false)}}
	} else if os.IsNotExist(err) {
		state.Headline = uiText("settings.linux.welcome_to_try_omarchy")
		state.Status = uiText("settings.linux.a_separate_omarchy_desktop_on_your_linux_computer")
		state.Detail = uiTextWith("settings.linux.about_to_download_and_of_free_space_needed", map[string]string{"download_bytes": linuxGB(linuxGuestDownloadBytes), "space_bytes": linuxGB(linuxGuestSpaceBytes)})
		state.Sections = []linuxSection{{Heading: uiText("launcher.linux.what_setup_does"), Rows: linuxSetupRows(dir, defaultDir, free)}}
	} else {
		state.Headline = uiText("settings.linux.omarchy_s_storage_cannot_be_read")
		state.Status = uiTextWith("settings.linux.cannot_read_virtual_machine_storage", map[string]string{"error": err.Error()})
	}
	integrations := linuxHomeIntegrationRows(dir)
	state.Sections = append(state.Sections, linuxSection{Heading: uiText("settings.linux.integrations"), Rows: integrations})
	settingsUnavailable := false
	for _, row := range integrations {
		settingsUnavailable = settingsUnavailable || row.State == "unavailable"
	}
	if err := linuxKVMCheck(); err != nil {
		state.Notice, state.HelpURL, state.CheckAgain = uiText("settings.linux.kvm_is_not_available"), linuxHelpURL("kvm"), true
		var kvm *kvmError
		if errors.As(err, &kvm) {
			state.Notice = kvm.Short
		}
		state.Headline, state.Status = uiText("settings.linux.omarchy_cannot_start_yet"), capitalizeFirst(err.Error())+"."
	} else if settingsUnavailable {
		state.Notice, state.HelpURL, state.CheckAgain = uiText("settings.linux.saved_settings_could_not_be_read"), linuxHelpURL("settings"), true
		state.Headline = uiText("settings.linux.saved_settings_need_attention")
		state.Status = uiText("settings.linux.open_settings_for_details_you_may_need_to")
	} else if free >= 0 && state.Installed && free < linuxLowSpaceBytes {
		state.Notice, state.HelpURL = uiTextWith("settings.linux.only_is_free_on_this_drive", map[string]string{"free": linuxGB(free)}), linuxHelpURL("space")
		state.Status = uiText("settings.linux.omarchy_can_stop_working_if_the_drive_fills")
	} else if free >= 0 && !state.Installed && free < linuxGuestSpaceBytes {
		state.Notice, state.HelpURL = uiTextWith("settings.linux.only_is_free_here", map[string]string{"free": linuxGB(free)}), linuxHelpURL("space")
		state.Status = uiTextWith("settings.linux.setup_needs_about_free_some_space_or_choose", map[string]string{"space_bytes": linuxGB(linuxGuestSpaceBytes)})
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
		return []linuxRow{{Title: uiText("launcher.linux.settings"), Detail: uiText("settings.linux.saved_choices_could_not_be_read_open_settings"), State: "unavailable"}}
	}
	resources, resourceErr := loadResourcePreferences(dir)
	desktop, deviceErr := loadDesktopPreferences(dir)
	_, audioErr := loadAudioPreferences(dir)
	_, storageErr := loadStorageSettings(dir)
	_, experienceErr := loadLinuxExperiencePreferences(dir)
	if resourceErr != nil || deviceErr != nil || audioErr != nil || storageErr != nil || experienceErr != nil {
		return []linuxRow{{Title: uiText("launcher.linux.settings"), Detail: uiText("settings.linux.saved_choices_could_not_be_read_open_settings"), State: "unavailable"}}
	}
	profile := effectiveResourceProfile(resources.Profile, prefs.CPUs, prefs.MemoryMiB)
	profileName := map[string]string{resourceBalanced: uiText("settings.resources.balanced"), resourceMaximum: uiText("settings.resources.maximum"), resourceManual: uiText("settings.resources.manual")}[profile]
	rows := []linuxRow{{Title: uiText("settings.section.resources"), Detail: uiTextWith("settings.linux.checked_at_the_next_launch", map[string]string{"profile_name": profileName}), State: "pending"}}
	share := linuxRow{Title: uiText("settings.linux.shared_folder"), Detail: uiText("settings.linux.not_shared"), State: "disabled"}
	if prefs.Share != "" && !prefs.ShareDisabled {
		share.Detail, share.State = uiTextWith("settings.linux.shared_at_the_next_launch", map[string]string{"path": linuxSharedFolderDisplayPath(prefs.Share)}), "enabled"
	}
	rows = append(rows, share)
	for _, device := range []struct {
		title    string
		disabled bool
	}{{uiText("settings.sound.microphone"), desktop.MicrophoneDisabled}, {uiText("settings.camera.camera"), desktop.CameraDisabled}} {
		r := linuxRow{Title: device.title, Detail: uiText("settings.linux.access_allowed_at_the_next_launch"), State: "enabled"}
		if device.disabled {
			r.Detail, r.State = uiText("settings.linux.access_disabled_at_the_next_launch"), "disabled"
		}
		if device.title == uiText("settings.camera.camera") && !device.disabled {
			r.Detail += uiText("settings.linux.your_desktop_asks_for_permission_when_needed")
		}
		rows = append(rows, r)
	}
	return rows
}

// linuxSetupRows is what a first setup will do, in the order it matters:
// what is downloaded, what it costs in space, where it goes and who you are.
func linuxSetupRows(dir, defaultDir string, free int64) []linuxRow {
	space := uiTextWith("setup.linux.about_omarchy_sees_a_disk_but_only_what", map[string]string{"space_bytes": linuxGB(linuxGuestSpaceBytes), "capacity": linuxGB(int64(24) << 30)})
	if free >= 0 {
		space += uiTextWith("settings.linux.is_free_here", map[string]string{"free": linuxGB(free)})
	}
	location := linuxStorageRows(dir, defaultDir, false)[0]
	return []linuxRow{
		{Title: uiText("settings.linux.download"), Detail: uiTextWith("settings.linux.about_once_if_it_is_interrupted_it_continues", map[string]string{"download_bytes": linuxGB(linuxGuestDownloadBytes)})},
		{Title: uiText("setup.linux.space"), Detail: space},
		location,
		{Title: uiText("settings.linux.account"), Detail: uiTextWith("settings.linux.your_own_username_and_password_or_a_quick", map[string]string{"username": trialUsername, "password": trialPassword})},
	}
}

// linuxHomeActions decides the home's buttons for its state. Storage that
// cannot be opened offers the way back to it first; a computer without KVM
// offers to check again; a first setup offers one button that just works.
func linuxHomeActions(state linuxSetupState) (actions, menu []linuxAction) {
	more := []linuxAction{{Label: uiText("settings.linux.about_and_help"), Reply: "about"}}
	if state.CanAttach {
		more = append(more, linuxAction{Label: uiText("settings.linux.use_existing_data_folder"), Reply: "attach"})
	}
	if state.CanDelete {
		more = append(more, linuxAction{Label: uiText("settings.linux.delete_this_vm"), Reply: "delete-default", Destructive: true})
	}
	switch {
	case state.CanForget:
		actions = []linuxAction{{Label: uiText("settings.linux.use_existing_data_folder"), Reply: "attach", Suggested: true}, {Label: uiText("launcher.linux.forget_saved_location_2"), Reply: "forget"}, {Label: uiText("settings.linux.about_and_help"), Reply: "about"}, {Label: uiText("launcher.close"), Reply: "close"}}
		return actions, nil
	case state.CheckAgain:
		actions = append(actions, linuxAction{Label: uiText("install.button.check"), Reply: "check", Suggested: true})
	case !state.Installed && state.Setup == "customize":
		actions = append(actions, linuxAction{Label: uiText("launcher.linux.choose_another_folder"), Reply: "customize", Suggested: true})
	case !state.Installed:
		actions = append(actions, linuxAction{Label: uiText("settings.linux.set_up_omarchy"), Reply: "setup", Suggested: true}, linuxAction{Label: uiText("launcher.linux.choose_location"), Reply: "customize"})
	default:
		actions = append(actions, linuxAction{Label: uiText("launcher.launch"), Reply: "launch", Suggested: true})
	}
	actions = append(actions, linuxAction{Label: uiText("launcher.linux.settings"), Reply: "settings"})
	if state.Installed {
		actions = append(actions, linuxAction{Label: uiText("settings.section.recovery"), Reply: "recovery"})
	}
	actions = append(actions, linuxAction{Label: uiText("launcher.close"), Reply: "close"})
	return actions, more
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func showLinuxHome(defaultDir, requestedDir string, explicitDir bool, autostart bool) bool {
	w := startLinuxWindow(func() {
		if linuxRecoveryActive.Load() {
			requestSetupCancel()
		}
	})
	if w == nil {
		// Login never boots a VM when the home helper is unavailable.
		// Ordinary launches keep the terminal/flag workflow available.
		return !autostart
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
				state = linuxSetupState{Prompt: "home", Status: uiTextWith("settings.linux.cannot_read_the_data_folder_move_record", map[string]string{"error": err.Error()})}
				dir = ""
			} else {
				state, dir = linuxHomeStateForDir(resolved, defaultDir)
			}
		}
		if dir != "" {
			tightenLinuxGuestData(dir)
		}
		// Finish an interrupted roll back before describing the VM; until then
		// its disk can be set aside and the home would offer a new setup.
		if dir != "" && linuxSnapshotRecoveryPending(dir) {
			if err := recoverLinuxSnapshots(dir); err != nil {
				logf("home: snapshot recovery: %v", err)
				state.Notice, state.HelpURL, state.CheckAgain = uiText("settings.linux.an_interrupted_roll_back_needs_attention"), linuxHelpURL("snapshots"), true
				state.Headline = uiText("settings.linux.omarchy_cannot_start_yet")
				state.Status = uiTextWith("settings.linux.try_omarchy_could_not_finish_rolling_back_to", map[string]string{"error": err.Error()})
			} else {
				state, dir = linuxHomeStateForDir(dir, defaultDir)
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
				if !state.CheckAgain {
					state.Notice, state.HelpURL, state.CheckAgain = uiText("settings.linux.startup_settings_could_not_be_read"), linuxHelpURL("settings"), true
					state.Headline = uiText("settings.linux.saved_settings_need_attention")
					if status == "" {
						state.Status = uiText("settings.linux.open_settings_for_details_fix_file_permissions_or")
					}
				}
			} else if launch.StartAutomatically && status == "" && state.Installed && !state.CheckAgain && !state.CanForget && state.Notice == "" {
				state.Status = uiText("settings.linux.omarchy_starts_automatically_in_10_seconds_choose_settings")
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
			state.Sections = append(state.Sections, linuxSection{Heading: uiText("settings.linux.integrations"), Rows: linuxHomeIntegrationRows(dir)})
			if err := linuxKVMCheck(); err != nil {
				status = ""
				continue
			}
			linuxQuickSetup.Store(answer == "setup")
			return true
		case "settings":
			if dir == "" {
				status = uiText("settings.linux.reconnect_or_repair_the_saved_storage_location_before")
				continue
			}
			status = showLinuxSettingsInWindow(context.Background(), w, dir, false)
		case "recovery":
			status = showLinuxRecoveryInWindow(w, defaultDir, dir)
		case "attach":
			selected, err := w.ask(context.Background(), linuxSetupState{Prompt: "attach-folder", Status: uiText("settings.linux.choose_an_existing_try_omarchy_data_folder_containing")})
			if err != nil || selected == "cancel" {
				status = ""
				continue
			}
			selected, err = attachLinuxDataDirectory(defaultDir, selected)
			if err != nil {
				status = uiTextWith("settings.linux.could_not_use_that_data_folder", map[string]string{"error": err.Error()})
			} else {
				status = uiTextWith("settings.linux.using_the_existing_vm_at_no_files_were", map[string]string{"path": selected})
			}
		case "forget":
			prompt := uiTextWith("settings.linux.forget_the_saved_location_at", map[string]string{"path": state.Path})
			if pathsEqual(state.Path, defaultDir) {
				prompt = uiTextWith("settings.linux.forget_the_unreadable_saved_location_record_in", map[string]string{"path": defaultDir})
			}
			answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "forget-location", Path: state.Path, Status: uiTextWith("settings.linux.this_only_removes_try_omarchy_s_location_choice", map[string]string{"prompt": prompt})})
			if err != nil || answer != "forget" {
				status = ""
				continue
			}
			if err := forgetLinuxDataLocation(defaultDir); err != nil {
				status = uiTextWith("settings.linux.could_not_forget_the_saved_location", map[string]string{"error": err.Error()})
			} else {
				status = uiText("settings.linux.saved_location_forgotten_its_files_were_not_deleted")
			}
		case "delete-default":
			answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "delete-default", Path: defaultDir, Status: uiTextWith("settings.linux.permanently_delete_this_vm_its_snapshots_and_its", map[string]string{"path": defaultDir})})
			if err != nil || answer != "delete" {
				status = uiText("settings.linux.the_vm_was_kept")
				continue
			}
			if err := deleteLinuxDefaultVM(defaultDir); err != nil {
				status = uiTextWith("settings.linux.could_not_delete_the_vm", map[string]string{"error": err.Error()})
			} else {
				status = uiTextWith("settings.linux.the_vm_its_snapshots_and_downloaded_guest_files", map[string]string{"path": defaultDir})
			}
		case "about":
			_, err := w.ask(context.Background(), linuxAboutState())
			if err != nil {
				return false
			}
		}
	}
}

// The document portal denies Info inside a sandbox. Keep the selected folder's
// recognizable name in that case; the exact grant remains in Share for access.
func linuxSharedFolderDisplayPath(path string) string {
	display := linuxDisplayPath(path)
	if display == path {
		if _, _, ok := linuxDocumentPath(path, os.Getuid()); ok {
			return filepath.Base(filepath.Clean(path))
		}
	}
	return display
}
