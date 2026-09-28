//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func linuxRecoveryTestWindow(t *testing.T, mode, folder, archive string) *linuxSetupWindow {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxSetupHelperProcess$")
	cmd.Env = append(os.Environ(), "TRY_OMARCHY_SETUP_TEST="+mode, "TRY_OMARCHY_RECOVERY_FOLDER="+folder, "TRY_OMARCHY_RECOVERY_ARCHIVE="+archive)
	w := launchLinuxWindow(cmd, func() { requestSetupCancel() })
	if w == nil {
		t.Fatal("recovery helper failed")
	}
	t.Cleanup(func() { w.stop(); configureSetupCancellation(false) })
	return w
}

func TestLinuxRecoveryWindowBackupAndRestoreAsCopy(t *testing.T) {
	dir, _ := backupFixture(t)
	backupFolder := filepath.Join(t.TempDir(), "backups")
	if err := os.Mkdir(backupFolder, 0700); err != nil {
		t.Fatal(err)
	}
	w := linuxRecoveryTestWindow(t, "linux-recovery-backup", backupFolder, "")
	result := showLinuxRecoveryInWindow(w, dir)
	if !strings.Contains(result, "Backup saved") {
		t.Fatalf("backup result: %s", result)
	}
	archives, err := filepath.Glob(filepath.Join(backupFolder, "try-omarchy-backup-*.zip"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("backup archive: %v, %v", archives, err)
	}
	w.stop()
	restoreParent := t.TempDir()
	w = linuxRecoveryTestWindow(t, "linux-recovery-restore", restoreParent, archives[0])
	result = showLinuxRecoveryInWindow(w, dir)
	if !strings.Contains(result, "Restored copy saved") {
		t.Fatalf("restore result: %s", result)
	}
	copies, err := filepath.Glob(filepath.Join(restoreParent, "try-omarchy-restored-*", "vm", "disk.raw"))
	if err != nil || len(copies) != 1 {
		t.Fatalf("restored disk: %v, %v", copies, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vm", "disk.raw")); err != nil {
		t.Fatalf("original was lost: %v", err)
	}
}
