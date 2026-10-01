//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxRecoveryMigrationReturnsToRecoveryWithoutChangingVM(t *testing.T) {
	dir := linuxSnapshotFixture(t)
	disk := filepath.Join(dir, "vm", "disk.raw")
	original, err := os.ReadFile(disk)
	if err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(t.TempDir(), "states.jsonl")
	t.Setenv("TRY_OMARCHY_STATE_RECORD", record)
	w := linuxScriptedWindow(t, "migration", "close", "back")
	if result := showLinuxRecoveryInWindow(w, dir, dir); result != "" {
		t.Fatal(result)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("expected recovery, migration and recovery, got %d prompts", len(lines))
	}
	for i, prompt := range []string{"recovery", "migration", "recovery"} {
		var sent linuxSetupState
		if err := json.Unmarshal(lines[i], &sent); err != nil {
			t.Fatal(err)
		}
		if sent.Prompt != prompt || (prompt == "recovery" && !sent.CanMigrate) {
			t.Fatalf("unexpected prompt: %+v", sent)
		}
		if prompt == "migration" {
			for _, want := range []string{"try-omarchy-export", "copy exists before installing", "./import.sh --dry-run", "restore.sh", "normal account"} {
				if !bytes.Contains(lines[i], []byte(want)) {
					t.Errorf("migration guidance missing %q", want)
				}
			}
		}
	}
	if got, err := os.ReadFile(disk); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("opening migration changed the VM: %v", err)
	}
}

func TestLinuxRecoveryDoesNotOfferMigrationForAnIncompleteVM(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(t.TempDir(), "states.jsonl")
	t.Setenv("TRY_OMARCHY_STATE_RECORD", record)
	showLinuxRecoveryInWindow(linuxScriptedWindow(t, "back"), dir, dir)
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var sent linuxSetupState
	if err := json.Unmarshal(bytes.TrimSpace(data), &sent); err != nil {
		t.Fatal(err)
	}
	if sent.CanMigrate {
		t.Fatal("incomplete VM offered migration")
	}
}

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
	result := showLinuxRecoveryInWindow(w, filepath.Join(t.TempDir(), "try-omarchy"), dir)
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
	result = showLinuxRecoveryInWindow(w, filepath.Join(t.TempDir(), "try-omarchy"), dir)
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
