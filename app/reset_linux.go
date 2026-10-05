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
		return uiText("recovery.linux.there_is_no_complete_vm_here_to_reset")
	}
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: uiText("recovery.linux.start_over_with_a_clean_vm"), Primary: uiText("recovery.linux.back_up_first"), Secondary: uiText("recovery.linux.reset_without_backup"), Destructive: true,
		Status: uiText("recovery.linux.resetting_replaces_the_omarchy_guest_with_a_clean")})
	if err != nil || (answer != "primary" && answer != "secondary") {
		return uiText("settings.linux.the_vm_was_kept")
	}
	if answer == "primary" {
		result, saved := backupLinuxVM(w, dir)
		if !saved {
			if result == "" {
				return uiText("settings.linux.the_vm_was_kept")
			}
			return uiTextWith("recovery.linux.the_vm_was_not_reset", map[string]string{"result": result})
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "guest", "build-spec.json"))
	if err != nil {
		return uiTextWith("recovery.linux.could_not_reset_the_vm", map[string]string{"error": err.Error()})
	}
	var spec buildSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return uiTextWith("recovery.linux.could_not_reset_the_vm", map[string]string{"error": err.Error()})
	}
	storage, err := loadStorageSettings(dir)
	if err != nil {
		return uiTextWith("recovery.linux.could_not_reset_the_vm", map[string]string{"error": err.Error()})
	}
	cfg := &config{dir: dir, guestDir: filepath.Join(dir, "guest"), vmDir: filepath.Join(dir, "vm"), disk: filepath.Join(dir, "vm", "disk.raw"), diskFormat: "raw", diskGiB: storage.DiskGiB}
	configureSetupCancellation(false)
	linuxRecoveryActive.Store(true)
	w.update(linuxSetupState{Status: uiText("recovery.linux.preparing_a_clean_omarchy_disk")})
	old, err := resetStandardDisk(cfg, spec.Runtime.Storage.ExpandedSizeMiB)
	linuxRecoveryActive.Store(false)
	configureSetupCancellation(false)
	if err != nil {
		return linuxRecoveryResult(err, "")
	}
	return uiTextWith("recovery.linux.the_vm_will_start_clean_at_its_next", map[string]string{"path": old})
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
	if err := validateMovePath(filepath.Join(dir, "vm")); err != nil {
		return err
	}
	for _, disk := range linuxRetainedResetDisks(dir) {
		entries, err := os.ReadDir(filepath.Dir(disk))
		if err != nil {
			return err
		}
		if len(entries) != 1 || entries[0].Name() != "disk.raw" {
			return uiError(uiTextWith("recovery.linux.the_kept_disk_folder_contains_unexpected_files_nothing", map[string]string{"path": filepath.Dir(disk)}), nil)
		}
		held, err := openBackupDisk(disk)
		if err != nil {
			return uiError(uiTextWith("recovery.linux.the_kept_disk_is_in_use", map[string]string{"error": fmt.Sprint(err)}), err)
		}
		if err := os.Remove(disk); err != nil {
			held.Close()
			return err
		}
		held.Close()
		if err := os.Remove(filepath.Dir(disk)); err != nil {
			return uiError(uiTextWith("recovery.linux.removed_the_kept_disk_but_not_its_folder", map[string]string{"error": fmt.Sprint(err)}), err)
		}
	}
	return nil
}

func cleanupLinuxResetDisks(w *linuxSetupWindow, dir string) string {
	disks := linuxRetainedResetDisks(dir)
	if len(disks) == 0 {
		return uiText("recovery.linux.there_is_no_disk_kept_from_a_reset")
	}
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: uiText("recovery.linux.remove_the_disk_kept_from_reset"), Primary: uiText("recovery.linux.keep_it"), Secondary: uiText("recovery.linux.remove_kept_disk"), Destructive: true,
		Status: uiTextWith("recovery.linux.permanently_remove_all_disks_kept_from_previous_resets", map[string]string{"names": strings.Join(disks, "\n")})})
	if err != nil || answer != "secondary" {
		return uiText("recovery.linux.the_kept_disk_was_not_removed")
	}
	if err := removeLinuxRetainedResetDisks(dir); err != nil {
		return uiTextWith("recovery.linux.could_not_remove_the_kept_disk", map[string]string{"error": err.Error()})
	}
	return uiText("recovery.linux.the_disk_kept_from_the_reset_was_removed")
}
