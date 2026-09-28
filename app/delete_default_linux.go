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

func checkLinuxDeleteFolder(dir string, allowed map[string]bool) ([]string, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is not a regular folder", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !allowed[name] {
			return nil, fmt.Errorf("%s contains an unrecognized file; nothing was deleted", dir)
		}
		info, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file; nothing was deleted", filepath.Join(dir, name))
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
		return fmt.Errorf("the default data folder is not a Try Omarchy folder")
	}
	if !linuxDefaultVMCanDelete(defaultDir) {
		return fmt.Errorf("only an app-owned default VM can be deleted here")
	}
	info, err := os.Lstat(defaultDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("the default data location is not a regular folder")
	}
	prefs, err := loadSettings(settingsPath(defaultDir))
	if err != nil {
		return fmt.Errorf("could not check the shared folder: %w", err)
	}
	if prefs.Share != "" && pathsOverlap(defaultDir, prefs.Share) {
		return fmt.Errorf("a shared folder overlaps this data location; no files were deleted")
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
			return fmt.Errorf("%s is not a regular file; nothing was deleted", mode)
		}
		modePresent = true
	} else if !os.IsNotExist(err) {
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
