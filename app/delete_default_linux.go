//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

var linuxVMDeleteFiles = map[string]bool{
	"disk.raw": true, "qemu.log": true, "qemu-stderr.log": true,
	"serial.log": true, "serial-gpu.log": true, "shell.log": true,
	checkpointBootFilename: true,
}

var linuxGuestDeleteFiles = map[string]bool{
	"build-spec.json": true, "guest-manifest.json": true,
	"install-state.json": true, "initramfs-linux.img": true,
	"rootfs.ext4": true, "rootfs.ext4.zst": true,
	"vmlinuz-linux": true,
}

func linuxDefaultVMCanDelete(defaultDir string) bool {
	_, found, err := loadDataLocationPointer(defaultDir)
	if err != nil || found {
		return false
	}
	info, err := os.Lstat(filepath.Join(defaultDir, "vm", "disk.raw"))
	return err == nil && info.Mode().IsRegular()
}

// linuxDeletePrompt names the folder and what deleting it frees. It says what
// is lost and what is kept, since Delete cannot be undone.
func linuxDeletePrompt(defaultDir string) string {
	freed := linuxTreeBytes(filepath.Join(defaultDir, "vm")) + linuxTreeBytes(filepath.Join(defaultDir, "guest")) + linuxTreeBytes(filepath.Join(defaultDir, "checkpoints"))
	return uiTextWith("recovery.linux.delete_this_vm_and_its_downloaded_system_files", map[string]string{"path": linuxDisplayPath(defaultDir), "freed": linuxGB(freed)})
}

func checkLinuxDeleteFolder(dir string, allowed map[string]bool) ([]string, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, uiError(uiTextWith("recovery.linux.is_not_a_regular_folder", map[string]string{"path": dir}), nil)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !allowed[name] {
			return nil, uiError(uiTextWith("recovery.linux.contains_an_unrecognized_file_nothing_was_deleted", map[string]string{"path": dir}), nil)
		}
		info, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, uiError(uiTextWith("recovery.linux.is_not_a_regular_file_nothing_was_deleted", map[string]string{"path": filepath.Join(dir, name)}), nil)
		}
		files = append(files, name)
	}
	return files, nil
}

// Delete only the built-in VM and downloaded guest files. Keep settings,
// diagnostics, backups and every shared folder. An external selected location
// cannot be deleted here, even if it happens to contain a complete VM.
func deleteLinuxDefaultVM(defaultDir string) error {
	defaultDir, err := validateDataLocationPath(defaultDir)
	if err != nil {
		return err
	}
	if filepath.Base(defaultDir) != "try-omarchy" {
		return uiError(uiText("recovery.linux.the_default_data_folder_is_not_a_try"), nil)
	}
	if !linuxDefaultVMCanDelete(defaultDir) {
		return uiError(uiText("recovery.linux.only_an_app_owned_default_vm_can_be"), nil)
	}
	info, err := os.Lstat(defaultDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return uiError(uiText("recovery.linux.the_default_data_location_is_not_a_regular"), nil)
	}
	prefs, err := loadSettings(settingsPath(defaultDir))
	if err != nil {
		return uiError(uiTextWith("recovery.linux.could_not_check_the_shared_folder", map[string]string{"error": fmt.Sprint(err)}), err)
	}
	if prefs.Share != "" && pathsOverlap(defaultDir, prefs.Share) {
		return uiError(uiText("recovery.linux.a_shared_folder_overlaps_this_data_location_no"), nil)
	}
	vmDir, guestDir := filepath.Join(defaultDir, "vm"), filepath.Join(defaultDir, "guest")
	vmFiles, err := checkLinuxDeleteFolder(vmDir, linuxVMDeleteFiles)
	if err != nil {
		return err
	}
	guestFiles, err := checkLinuxDeleteFolder(guestDir, linuxGuestDeleteFiles)
	if err != nil {
		return err
	}
	mode := filepath.Join(defaultDir, "provision-mode")
	modePresent := false
	if info, err := os.Lstat(mode); err == nil {
		if !info.Mode().IsRegular() {
			return uiError(uiTextWith("recovery.linux.is_not_a_regular_file_nothing_was_deleted_2", map[string]string{"mode": mode}), nil)
		}
		modePresent = true
	} else if !os.IsNotExist(err) {
		return err
	}
	disk, err := openBackupDisk(filepath.Join(vmDir, "disk.raw"))
	if err != nil {
		return uiError(uiTextWith("recovery.linux.the_guest_disk_is_in_use_or_could", map[string]string{"error": fmt.Sprint(err)}), err)
	}
	defer disk.Close()
	// Snapshots and the state a roll back kept are this VM's own copies.
	// Remove them first, so a failure leaves the VM itself in place.
	if err := removeLinuxSnapshotStore(defaultDir); err != nil {
		return uiError(uiTextWith("recovery.linux.could_not_remove_this_vm_s_snapshots", map[string]string{"error": fmt.Sprint(err)}), err)
	}
	if err := removeLinuxRollbackKept(defaultDir); err != nil {
		return err
	}
	for _, name := range vmFiles {
		if err := os.Remove(filepath.Join(vmDir, name)); err != nil {
			return err
		}
	}
	for _, name := range guestFiles {
		if err := os.Remove(filepath.Join(guestDir, name)); err != nil {
			return err
		}
	}
	if err := os.Remove(vmDir); err != nil {
		return err
	}
	if err := os.Remove(guestDir); err != nil {
		return err
	}
	if modePresent {
		if err := os.Remove(mode); err != nil {
			return err
		}
	}
	return nil
}
