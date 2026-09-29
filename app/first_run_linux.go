//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
)

// linuxQuickSetup is set when the home's Set up Omarchy was chosen: the default
// location without asking. The account question is still asked.
var linuxQuickSetup atomic.Bool

// A first setup needs room for the download, the unpacked system files and the
// VM's own copy of them. Say so before anything is downloaded, not halfway.
func linuxSetupSpaceError(dir string) error {
	free := linuxFreeBytes(dir)
	if free < 0 || free >= linuxGuestSpaceBytes {
		return nil
	}
	return fmt.Errorf("only %s is free where %s would go, and setup needs about %s. Free some space or choose another folder", linuxGB(free), linuxDisplayPath(dir), linuxGB(linuxGuestSpaceBytes))
}

func linuxLocationState(defaultDir, notice string) linuxSetupState {
	rows := []linuxRow{
		{Title: "Default location", Detail: linuxDisplayPath(defaultDir) + "\nInside the app's own storage."},
		{Title: "Space", Detail: "About " + linuxGB(linuxGuestSpaceBytes) + ". Omarchy sees a " + linuxGB(int64(24)<<30) + " disk, but only what it uses takes space."},
	}
	if free := linuxFreeBytes(defaultDir); free >= 0 {
		rows[0].Detail += " " + linuxGB(free) + " is free."
	}
	return linuxSetupState{Prompt: "location", Path: defaultDir, Notice: notice,
		Status:   "Omarchy lives in one folder on this computer. Use the default, or pick a bigger drive. You can move it later.",
		Sections: []linuxSection{{Rows: rows}}}
}

func linuxAccountState() linuxSetupState {
	return linuxSetupState{Prompt: "account", Status: "How do you want to start?",
		Sections: []linuxSection{{Rows: []linuxRow{
			{Title: "Set up my own account", Detail: "Omarchy's setup asks for your name and password when it first starts. Sudo and the lock screen use that password."},
			{Title: "Quick start as " + trialUsername, Detail: "Skips setup and signs you in automatically as " + trialUsername + " (password " + trialPassword + "). Sudo does not ask for a password. Good for a first look."},
		}}}}
}

// Explicit flags bypass saved choices. Remembered removable locations must
// still exist; never silently create a replacement VM after a drive disappears.
func resolveLinuxDataDirectory(defaultDir, requested string, explicit bool, choose dataLocationChooser) (string, bool, error) {
	if explicit {
		// Recovery in the idle home may have moved this folder since startup.
		resolved, err := resolveLinuxMovedDirectory(defaultDir, requested)
		if err != nil {
			return "", false, err
		}
		requested = resolved
	}
	if !explicit {
		saved, found, err := loadDataLocationPointer(defaultDir)
		if err != nil {
			return "", false, err
		}
		if found {
			info, err := os.Stat(saved)
			if err != nil {
				return "", false, fmt.Errorf("saved data folder %s is unavailable: %w; reconnect its drive and try again", saved, err)
			}
			if !info.IsDir() {
				return "", false, fmt.Errorf("saved data folder %s is not a directory", saved)
			}
		}
		// Settings saved from the idle home must not silently claim the default
		// storage location. Keep the location choice, and carry those preferences
		// to a newly selected folder before recording its pointer.
		if !found && choose != nil && linuxPrebootSettingsOnly(defaultDir) {
			selected, proceed, err := choose(defaultDir)
			if err != nil || !proceed {
				return selected, proceed, err
			}
			selected, err = validateDataLocationPath(selected)
			if err != nil {
				return "", false, err
			}
			if !pathsEqual(selected, defaultDir) {
				if _, err := os.Stat(settingsPath(selected)); os.IsNotExist(err) {
					prefs, err := loadSettings(settingsPath(defaultDir))
					if err != nil {
						return "", false, err
					}
					if prefs.Share != "" && !prefs.ShareDisabled {
						if _, err := validateLinuxSharedFolder(prefs.Share, selected); err != nil {
							return "", false, fmt.Errorf("selected storage conflicts with the saved shared folder: %w", err)
						}
					}
					if err := saveSettings(settingsPath(selected), prefs); err != nil {
						return "", false, err
					}
				} else if err != nil {
					return "", false, err
				}
				if _, err := os.Stat(filepath.Join(defaultDir, desktopPreferencesFilename)); err == nil {
					if _, err := os.Stat(filepath.Join(selected, desktopPreferencesFilename)); os.IsNotExist(err) {
						prefs, err := loadDesktopPreferences(defaultDir)
						if err != nil {
							return "", false, err
						}
						if err := saveDesktopPreferences(selected, prefs); err != nil {
							return "", false, err
						}
					} else if err != nil {
						return "", false, err
					}
				} else if !os.IsNotExist(err) {
					return "", false, err
				}
				if _, err := os.Stat(filepath.Join(defaultDir, audioPreferencesFilename)); err == nil {
					if _, err := os.Stat(filepath.Join(selected, audioPreferencesFilename)); os.IsNotExist(err) {
						prefs, err := loadAudioPreferences(defaultDir)
						if err != nil {
							return "", false, err
						}
						if err := saveAudioPreferences(selected, prefs); err != nil {
							return "", false, err
						}
					} else if err != nil {
						return "", false, err
					}
				} else if !os.IsNotExist(err) {
					return "", false, err
				}
				if _, err := os.Stat(filepath.Join(defaultDir, storageSettingsFilename)); err == nil {
					if _, err := os.Stat(filepath.Join(selected, storageSettingsFilename)); os.IsNotExist(err) {
						prefs, err := loadStorageSettings(defaultDir)
						if err != nil {
							return "", false, err
						}
						if err := saveStorageSettings(selected, prefs.DiskGiB); err != nil {
							return "", false, err
						}
					} else if err != nil {
						return "", false, err
					}
				} else if !os.IsNotExist(err) {
					return "", false, err
				}
				if _, err := os.Stat(filepath.Join(defaultDir, linuxExperiencePreferencesFilename)); err == nil {
					if _, err := os.Stat(filepath.Join(selected, linuxExperiencePreferencesFilename)); os.IsNotExist(err) {
						prefs, err := loadLinuxExperiencePreferences(defaultDir)
						if err != nil {
							return "", false, err
						}
						if err := saveLinuxExperiencePreferences(selected, prefs); err != nil {
							return "", false, err
						}
					} else if err != nil {
						return "", false, err
					}
				} else if !os.IsNotExist(err) {
					return "", false, err
				}
				if _, err := os.Stat(filepath.Join(defaultDir, launchPreferencesFilename)); err == nil {
					if _, err := os.Stat(filepath.Join(selected, launchPreferencesFilename)); os.IsNotExist(err) {
						prefs, err := loadLaunchPreferences(defaultDir)
						if err != nil {
							return "", false, err
						}
						if err := saveLaunchPreferences(selected, prefs); err != nil {
							return "", false, err
						}
					} else if err != nil {
						return "", false, err
					}
				} else if !os.IsNotExist(err) {
					return "", false, err
				}
				if err := saveDataLocationPointer(defaultDir, selected); err != nil {
					return "", false, err
				}
			}
			return selected, true, nil
		}
	}
	return resolveStandardDataDirectory(defaultDir, requested, explicit, choose != nil, choose)
}

func linuxPrebootSettingsOnly(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	found := false
	for _, entry := range entries {
		switch entry.Name() {
		case settingsFileName:
			found = true
		case desktopPreferencesFilename, audioPreferencesFilename, storageSettingsFilename, linuxExperiencePreferencesFilename, launchPreferencesFilename, "diagnostics", "portable-host":
		default:
			return false
		}
	}
	return found
}

func (u *progressUI) chooseLocation(defaultDir string) (string, bool, error) {
	notice := ""
	// Set up Omarchy takes the default without asking, but it still has to pass the
	// checks below. If it does not, the questions come back with the reason.
	ask := !linuxQuickSetup.Load()
	for {
		value := "default"
		if ask {
			var err error
			value, err = u.window.ask(setupContext(), linuxLocationState(defaultDir, notice))
			if err != nil {
				return "", false, err
			}
		}
		ask = true
		selected := defaultDir
		var err error
		if value != "default" {
			selected, err = validateDataLocationPath(value)
			if err == nil && filepath.Base(selected) != "try-omarchy" {
				selected = filepath.Join(selected, "try-omarchy")
			}
		}
		if err == nil {
			var selectable bool
			selectable, err = linuxDataLocationSelectable(selected, defaultDir)
			if err == nil && !selectable {
				err = fmt.Errorf("%s already contains other files or an incomplete installation. Choose an empty folder, or use -dir to resume that installation", linuxDisplayPath(selected))
			}
		}
		if err == nil {
			prefs, loadErr := loadSettings(settingsPath(defaultDir))
			if loadErr != nil {
				err = loadErr
			} else if prefs.Share != "" && !prefs.ShareDisabled {
				_, err = validateLinuxSharedFolder(prefs.Share, selected)
			}
		}
		if err == nil {
			err = linuxSetupSpaceError(selected)
		}
		if err == nil {
			err = ensureDataDirectoryWritable(selected)
		}
		if err == nil {
			return selected, true, nil
		}
		linuxQuickSetup.Store(false)
		notice = "Cannot use that folder: " + err.Error()
	}
}

func linuxDataLocationSelectable(selected, defaultDir string) (bool, error) {
	// Saving preferences on the idle home writes only settings.json. That
	// folder is still an unclaimed first-run location.
	if pathsEqual(selected, defaultDir) && linuxPrebootSettingsOnly(defaultDir) {
		return true, nil
	}
	return standardDataDirectorySelectable(selected)
}

// Reattach only changes the launcher's location pointer after an existing VM
// and its boot files are present and writable. It does not move or delete data.
func attachLinuxDataDirectory(defaultDir, selected string) (string, error) {
	defaultDir, err := validateDataLocationPath(defaultDir)
	if err != nil {
		return "", err
	}
	selected, err = validateDataLocationPath(selected)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(selected)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("the selected location is not a folder")
	}
	required := []string{"vm/disk.raw", "guest/build-spec.json", "guest/rootfs.ext4", "guest/vmlinuz-linux", "guest/initramfs-linux.img"}
	for _, name := range required {
		info, err := os.Lstat(filepath.Join(selected, filepath.FromSlash(name)))
		if err != nil || !info.Mode().IsRegular() {
			return "", fmt.Errorf("the selected folder has no complete Try Omarchy VM: %s", name)
		}
	}
	// The drive can disappear after validation. Probe only the existing folder;
	// the first-run helper would create a replacement directory in that case.
	probe, err := os.CreateTemp(selected, ".tryomarchy-write-test-*")
	if err != nil {
		return "", fmt.Errorf("the selected data folder is not writable: %w", err)
	}
	probeName := probe.Name()
	if err := probe.Close(); err != nil {
		os.Remove(probeName)
		return "", err
	}
	if err := os.Remove(probeName); err != nil {
		return "", err
	}
	// Do not publish a pointer if a removable location changed during the probe.
	for _, name := range required {
		info, err := os.Lstat(filepath.Join(selected, filepath.FromSlash(name)))
		if err != nil || !info.Mode().IsRegular() {
			return "", fmt.Errorf("the selected folder changed before it could be attached: %s", name)
		}
	}
	if pathsEqual(defaultDir, selected) {
		err = os.Remove(dataLocationPointerPath(defaultDir))
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	} else if err := saveDataLocationPointer(defaultDir, selected); err != nil {
		return "", err
	}
	return selected, nil
}

// Forgetting an unavailable location removes only the launcher's pointer.
// The selected VM and every shared folder remain untouched, even if the drive
// is reconnected later.
func forgetLinuxDataLocation(defaultDir string) error {
	defaultDir, err := validateDataLocationPath(defaultDir)
	if err != nil {
		return err
	}
	path := dataLocationPointerPath(defaultDir)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("saved location record is not a regular file")
	}
	return os.Remove(path)
}

// An existing disk without a mode predates this flow. Leave its account alone.
// Fresh disks get a new choice; interrupted setup resumes its saved choice.
func chooseLinuxProvisionMode(cfg *config, explicit bool, choose func() (string, error)) error {
	if !cfg.fresh && !explicit {
		if mode, found := readProvisionMode(cfg.dir); found {
			cfg.instant = mode == provisionModeInstant
			return nil
		}
	}
	if !cfg.fresh {
		if _, err := os.Stat(cfg.disk); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	mode := provisionModePersonal
	if cfg.instant {
		mode = provisionModeInstant
	}
	if !explicit && choose != nil {
		var err error
		mode, err = choose()
		if err != nil {
			return err
		}
	}
	if err := writeProvisionMode(cfg.dir, mode); err != nil {
		return err
	}
	cfg.instant = mode == provisionModeInstant
	return nil
}
