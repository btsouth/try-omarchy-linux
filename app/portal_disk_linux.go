//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The document portal's FUSE filesystem does not implement QEMU's OFD locks.
// The lifecycle listener has already claimed the single-instance port before
// any data-folder or disk operation. Disable QEMU locking only on this mount.
func linuxQemuArgs(cfg *config, args []string) []string {
	args = linuxGraphicsArgs(args, linuxVenusEnabled, linuxHonorGuestPAT)
	args = linuxAudioArgs(cfg, args)
	args = linuxWithoutWindowsHello(args)
	if !linuxDocumentPortalDisk(cfg.disk, os.Getuid()) {
		return args
	}
	disk := "file=" + qemuOptionValue(cfg.disk) + ",format=" + cfg.diskFormat + ",if=virtio"
	for i := 1; i < len(args); i++ {
		if args[i-1] == "-drive" && args[i] == disk {
			args[i] += ",file.locking=off"
		}
	}
	return args
}

// A private desktop session can place the Documents mount below another
// directory in the user's runtime tree. Keep the check inside that tree and
// require a document grant after the doc component.
func linuxDocumentPortalDisk(disk string, uid int) bool {
	runtimeRoot := fmt.Sprintf("/run/user/%d", uid)
	rel, err := filepath.Rel(runtimeRoot, filepath.Clean(disk))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	for i, part := range parts {
		if part == "doc" && i+2 < len(parts) && parts[i+1] != "" {
			return true
		}
	}
	return false
}
