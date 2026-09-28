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
	AudioOutput        string             `json:"audioOutput"`
	AudioInput         string             `json:"audioInput"`
	AudioOutputs       []linuxAudioDevice `json:"audioOutputs,omitempty"`
	AudioInputs        []linuxAudioDevice `json:"audioInputs,omitempty"`
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
	CPUMax             int                `json:"cpuMax"`
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

// The home and running VM use the same form. Saving while QEMU is running
// changes the next launch, never the live process or a guest reboot.
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
	outputs, inputs, audioListErr := listLinuxAudioDevices()
	sshEnabled, sshPort, additionalForwards := linuxNetworkForm(saved.Forwards)
	form := &linuxSettingsForm{Memory: strconv.Itoa(saved.MemoryMiB), CPUs: strconv.Itoa(saved.CPUs), Render: saved.Render, Fullscreen: saved.Fullscreen, Microphone: !desktop.MicrophoneDisabled, AudioOutput: audio.Output, AudioInput: audio.Input, AudioOutputs: outputs, AudioInputs: inputs, DiskGiB: strconv.Itoa(storage.DiskGiB), Scale: experience.Scale, Keyboard: experience.Keyboard, SSHEnabled: sshEnabled, SSHPort: sshPort, SSHKey: saved.SSHKey, Forwards: additionalForwards, StartAutomatically: launch.StartAutomatically, Share: saved.Share, ShareEnabled: saved.Share != "" && !saved.ShareDisabled, CPUMax: min(maximumGuestCPUs, max(1, measureHostResources(false).LogicalCPUs))}
	if form.Render == "" {
		form.Render = "auto"
	}
	status := "Changes apply when the VM starts."
	if running {
		status = "Saved changes require shutting down Omarchy and launching it again. Rebooting inside Omarchy will not apply host settings."
	}
	if audioListErr != nil {
		status += " Audio devices could not be listed. System default remains available."
	}
	if linuxGNOMEWayland() {
		status += " Clipboard sync asks GNOME for pointer and clipboard permission. Try Omarchy never sends pointer events; you can decline and still drop files."
	}
	for {
		value, err := w.ask(ctx, linuxSetupState{Prompt: "settings", Status: status, Settings: form})
		if err != nil || value == "cancel" {
			return ""
		}
		err = json.Unmarshal([]byte(value), form)
		if err != nil {
			status = "Could not read the settings."
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
			if err == nil && desktop.MicrophoneDisabled == form.Microphone {
				nextDesktop := desktop
				nextDesktop.MicrophoneDisabled = !form.Microphone
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
			if err == nil && form.StartAutomatically != launch.StartAutomatically {
				nextLaunch := launch
				nextLaunch.StartAutomatically = form.StartAutomatically
				err = saveLaunchPreferences(dir, nextLaunch)
				if err == nil {
					launch = nextLaunch
				}
			}
		}
		if err == nil {
			if running {
				message := "Settings saved. Shut down Omarchy and launch it again to apply them. A guest reboot does not restart the VM."
				w.ask(ctx, linuxSetupState{Prompt: "settings-saved", Status: message})
				return message
			}
			return "Settings saved. They will apply when you launch Omarchy."
		}
		status = err.Error()
	}
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

func linuxHomeState(defaultDir string) (linuxSetupState, string) {
	dir := defaultDir
	if saved, found, err := loadDataLocationPointer(defaultDir); err != nil {
		return linuxSetupState{Prompt: "home", Status: "Saved storage choice could not be read. Choose an existing data folder, or forget this choice to start fresh: " + err.Error(), Path: defaultDir, Version: linuxAppVersion, CanForget: true}, ""
	} else if found {
		dir = saved
		if _, err := os.Stat(dir); err != nil {
			return linuxSetupState{Prompt: "home", Status: "Saved storage is unavailable. Reconnect its drive, choose an existing data folder, or forget this choice to start fresh: " + err.Error(), Path: dir, Version: linuxAppVersion, CanForget: true}, ""
		}
	}
	return linuxHomeStateForDir(dir)
}

func linuxHomeStateForDir(dir string) (linuxSetupState, string) {
	status := "No virtual machine is installed yet. Launch to choose its storage location and set it up."
	if info, err := os.Stat(filepath.Join(dir, "vm", "disk.raw")); err == nil {
		status = fmt.Sprintf("Saved virtual machine disk found. Capacity: %.0f GiB. Launch to check its system files and start Omarchy.", float64(info.Size())/1073741824)
	} else if !os.IsNotExist(err) {
		status = "Cannot read virtual machine storage: " + err.Error()
	}
	return linuxSetupState{Prompt: "home", Status: status, Path: dir, Version: linuxAppVersion}, dir
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
	status := ""
	for {
		state, dir := linuxHomeState(defaultDir)
		if explicitDir {
			resolved, err := resolveLinuxMovedDirectory(defaultDir, requestedDir)
			if err != nil {
				state = linuxSetupState{Prompt: "home", Status: "Cannot read the data folder move record: " + err.Error()}
				dir = ""
			} else {
				state, dir = linuxHomeStateForDir(resolved)
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
		answer, err := w.ask(ctx, state)
		cancel()
		if errors.Is(err, context.DeadlineExceeded) {
			return true
		}
		if err != nil || answer == "close" {
			return false
		}
		switch answer {
		case "launch":
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
			_, err := w.ask(context.Background(), linuxSetupState{Prompt: "about", Version: linuxAppVersion})
			if err != nil {
				return false
			}
		}
	}
}
