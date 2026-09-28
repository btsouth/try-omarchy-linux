//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const linuxResetRetainedPrefix = "before-reset-"

func resetLinuxVM(w *linuxSetupWindow, dir string) string {
	if !completeInstallExists(dir, "disk.raw") {
		return "There is no complete VM here to reset."
	}
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: "Start over with a clean VM?", Primary: "Back up first", Secondary: "Reset without backup", Destructive: true,
		Status: "Resetting replaces the Omarchy guest with a clean one: its account, installed apps and files inside Omarchy. Settings and shared folders are kept, and the next launch sets up the same kind of account as before.\n\nThe current disk is kept in this VM's folder until you remove it from Backup and recovery."})
	if err != nil || (answer != "primary" && answer != "secondary") {
		return "The VM was kept."
	}
	if answer == "primary" {
		result, saved := backupLinuxVM(w, dir)
		if !saved {
			if result == "" {
				return "The VM was kept."
			}
			return result + " The VM was not reset."
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "guest", "build-spec.json"))
	if err != nil {
		return "Could not reset the VM: " + err.Error()
	}
	var spec buildSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return "Could not reset the VM: " + err.Error()
	}
	storage, err := loadStorageSettings(dir)
	if err != nil {
		return "Could not reset the VM: " + err.Error()
	}
	cfg := &config{dir: dir, guestDir: filepath.Join(dir, "guest"), vmDir: filepath.Join(dir, "vm"), disk: filepath.Join(dir, "vm", "disk.raw"), diskFormat: "raw", diskGiB: storage.DiskGiB}
	configureSetupCancellation(false)
	linuxRecoveryActive.Store(true)
	w.update(linuxSetupState{Status: "Preparing a clean Omarchy disk..."})
	old, err := resetStandardDisk(cfg, spec.Runtime.Storage.ExpandedSizeMiB)
	linuxRecoveryActive.Store(false)
	configureSetupCancellation(false)
	if err != nil {
		return linuxRecoveryResult(err, "")
	}
	return "The VM will start clean at its next launch. The previous disk is kept at " + old + " until you remove it from Backup and recovery."
}

// linuxRetainedResetDisks lists the disks kept by earlier resets, oldest first.
func linuxRetainedResetDisks(dir string) []string {
	entries, err := os.ReadDir(filepath.Join(dir, "vm"))
	if err != nil {
		return nil
	}
	var disks []string
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), linuxResetRetainedPrefix) {
			continue
		}
		disk := filepath.Join(dir, "vm", entry.Name(), "disk.raw")
		if info, err := os.Lstat(disk); err == nil && info.Mode().IsRegular() {
			disks = append(disks, disk)
		}
	}
	sort.Strings(disks)
	return disks
}

// Only a regular disk.raw inside a before-reset-* folder is removed, together
// with that folder once it is empty. Anything unexpected stops the removal.
func removeLinuxRetainedResetDisks(dir string) error {
	for _, disk := range linuxRetainedResetDisks(dir) {
		held, err := openBackupDisk(disk)
		if err != nil {
			return fmt.Errorf("the kept disk is in use: %w", err)
		}
		held.Close()
		if err := os.Remove(disk); err != nil {
			return err
		}
		if err := os.Remove(filepath.Dir(disk)); err != nil {
			return fmt.Errorf("removed the kept disk but not its folder: %w", err)
		}
	}
	return nil
}

func cleanupLinuxResetDisks(w *linuxSetupWindow, dir string) string {
	disks := linuxRetainedResetDisks(dir)
	if len(disks) == 0 {
		return "There is no disk kept from a reset."
	}
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: "Remove the disk kept from reset?", Primary: "Keep it", Secondary: "Remove kept disk", Destructive: true,
		Status: "Permanently remove the Omarchy disk kept from the last reset? Files inside it will be lost. The current VM is not affected.\n\n" + strings.Join(disks, "\n")})
	if err != nil || answer != "secondary" {
		return "The kept disk was not removed."
	}
	if err := removeLinuxRetainedResetDisks(dir); err != nil {
		return "Could not remove the kept disk: " + err.Error()
	}
	return "The disk kept from the reset was removed."
}
