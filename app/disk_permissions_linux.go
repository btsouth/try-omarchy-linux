//go:build linux

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Older writable disks and retained copies may have inherited the host umask.
// Modes are best effort for removable drives and document portal mounts.
func tightenLinuxGuestData(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		name := entry.Name()
		if name != "vm" && name != "checkpoints" && !linuxRollbackStage.MatchString(name) && !strings.HasPrefix(name, ".snapshot-preparing-") {
			continue
		}
		_ = filepath.WalkDir(filepath.Join(dir, name), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				_ = os.Chmod(path, 0o700)
			} else if entry.Type().IsRegular() {
				_ = os.Chmod(path, 0o600)
			}
			return nil
		})
	}
}
