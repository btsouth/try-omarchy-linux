//go:build linux

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func linuxScriptedWindow(t *testing.T, replies ...string) *linuxSetupWindow {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxSetupHelperProcess$")
	cmd.Env = append(os.Environ(), "TRY_OMARCHY_SETUP_TEST=linux-recovery-script", "TRY_OMARCHY_RECOVERY_REPLIES="+strings.Join(replies, "\n"))
	w := launchLinuxWindow(cmd, func() { requestSetupCancel() })
	if w == nil {
		t.Fatal("scripted window failed")
	}
	t.Cleanup(func() { w.stop(); configureSetupCancellation(false) })
	return w
}

func TestLinuxMoveDefaultVMThroughRecoveryWindow(t *testing.T) {
	source, _ := backupFixture(t)
	defaultDir := source
	original, err := os.ReadFile(filepath.Join(source, "vm", "disk.raw"))
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	destination := filepath.Join(parent, "try-omarchy")

	result := showLinuxRecoveryInWindow(linuxScriptedWindow(t, "move", parent, "primary"), defaultDir, source)
	if !strings.Contains(result, "now lives at "+destination) {
		t.Fatalf("move result: %s", result)
	}
	moved, err := os.ReadFile(filepath.Join(destination, "vm", "disk.raw"))
	if err != nil || !bytes.Equal(moved, original) {
		t.Fatalf("moved disk differs: %v", err)
	}
	if selected, found, err := loadDataLocationPointer(defaultDir); err != nil || !found || !pathsEqual(selected, destination) {
		t.Fatalf("default location not repointed: %q %v %v", selected, found, err)
	}
	if _, err := os.Stat(filepath.Join(source, "vm", "disk.raw")); err != nil {
		t.Fatalf("original was not retained: %v", err)
	}
	if resolved, err := resolveLinuxMovedDirectory(defaultDir, source); err != nil || !pathsEqual(resolved, destination) {
		t.Fatalf("explicit -dir of the original is not redirected: %q %v", resolved, err)
	}

	// Removal waits for the moved copy to boot.
	if retained, booted := linuxRetainedMove(defaultDir, destination); retained == nil || booted {
		t.Fatalf("retained before boot: %v %v", retained, booted)
	}
	if result := cleanupLinuxMove(linuxScriptedWindow(t), defaultDir, destination); !strings.Contains(result, "Start the moved VM once") {
		t.Fatalf("cleanup before boot: %s", result)
	}
	markLinuxMovedGuestReady(defaultDir, destination)
	if _, booted := linuxRetainedMove(defaultDir, destination); !booted {
		t.Fatal("moved boot was not recorded")
	}
	result = showLinuxRecoveryInWindow(linuxScriptedWindow(t, "clean-move", "secondary"), defaultDir, destination)
	if !strings.Contains(result, "previous copy was removed") {
		t.Fatalf("cleanup result: %s", result)
	}
	if _, err := os.Stat(filepath.Join(source, "vm", "disk.raw")); !os.IsNotExist(err) {
		t.Fatalf("original disk still present: %v", err)
	}
	if selected, found, _ := loadDataLocationPointer(defaultDir); !found || !pathsEqual(selected, destination) {
		t.Fatal("cleanup removed the location record")
	}
	if retained, _ := linuxRetainedMove(defaultDir, destination); retained != nil {
		t.Fatal("retained record survived cleanup")
	}
}

func TestLinuxMoveDeclinedLeavesNoTrace(t *testing.T) {
	source, _ := backupFixture(t)
	parent := t.TempDir()
	if result := showLinuxRecoveryInWindow(linuxScriptedWindow(t, "move", parent, "secondary"), source, source); result != "" {
		t.Fatalf("declined move result: %s", result)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("declined move wrote files: %v %v", entries, err)
	}
	if _, found, _ := loadDataLocationPointer(source); found {
		t.Fatal("declined move changed the location")
	}
}

func TestLinuxLaunchAfterExplicitMoveUsesDestination(t *testing.T) {
	source, _ := backupFixture(t)
	defaultDir := filepath.Join(t.TempDir(), "default")
	parent := t.TempDir()
	destination := filepath.Join(parent, "try-omarchy")
	result := showLinuxRecoveryInWindow(linuxScriptedWindow(t, "move", parent, "primary"), defaultDir, source)
	if !strings.Contains(result, "now lives at "+destination) {
		t.Fatal(result)
	}
	selected, proceed, err := resolveLinuxDataDirectory(defaultDir, source, true, nil)
	if err != nil || !proceed || !pathsEqual(selected, destination) {
		t.Fatalf("launch after move selected %q, proceed=%v, error=%v", selected, proceed, err)
	}
	if _, found, err := loadDataLocationPointer(defaultDir); err != nil || found {
		t.Fatalf("explicit move changed the default selection: %v %v", found, err)
	}
}

func TestLinuxResetCleanupPreservesUnexpectedContents(t *testing.T) {
	dir := resetReadyFixture(t)
	kept := filepath.Join(dir, "vm", "before-reset-fixture")
	if err := os.Mkdir(kept, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"disk.raw", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(kept, name), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeLinuxRetainedResetDisks(dir); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("unexpected contents accepted: %v", err)
	}
	for _, name := range []string{"disk.raw", "notes.txt"} {
		if data, err := os.ReadFile(filepath.Join(kept, name)); err != nil || string(data) != "keep" {
			t.Fatalf("kept file %s changed: %q %v", name, data, err)
		}
	}
}

func TestLinuxResetCleanupRefusesLinkedVMDirectory(t *testing.T) {
	dir := t.TempDir()
	external := t.TempDir()
	kept := filepath.Join(external, "before-reset-fixture")
	if err := os.Mkdir(kept, 0700); err != nil {
		t.Fatal(err)
	}
	disk := filepath.Join(kept, "disk.raw")
	if err := os.WriteFile(disk, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(dir, "vm")); err != nil {
		t.Fatal(err)
	}
	if err := removeLinuxRetainedResetDisks(dir); err == nil {
		t.Fatal("linked VM directory accepted")
	}
	if data, err := os.ReadFile(disk); err != nil || string(data) != "keep" {
		t.Fatalf("external disk changed: %q %v", data, err)
	}
}

func TestLinuxMoveRefusesNonEmptyDestination(t *testing.T) {
	source, _ := backupFixture(t)
	parent := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parent, "try-omarchy"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "try-omarchy", "other.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	result := showLinuxRecoveryInWindow(linuxScriptedWindow(t, "move", parent, "primary"), source, source)
	if !strings.Contains(result, "not empty") {
		t.Fatalf("non-empty destination result: %s", result)
	}
	if data, err := os.ReadFile(filepath.Join(parent, "try-omarchy", "other.txt")); err != nil || string(data) != "keep" {
		t.Fatal("existing destination file changed")
	}
	if _, found, _ := loadDataLocationPointer(source); found {
		t.Fatal("failed move changed the location")
	}
}

func resetReadyFixture(t *testing.T) string {
	t.Helper()
	dir, _ := backupFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "guest", "build-spec.json"), []byte(`{"runtime":{"storage":{"expandedSizeMiB":1}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vm", "disk.raw"), []byte("personal files"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, storageSettingsFilename), []byte(`{"schemaVersion":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLinuxResetWithBackupThenRemoveKeptDisk(t *testing.T) {
	dir := resetReadyFixture(t)
	backups := t.TempDir()
	result := showLinuxRecoveryInWindow(linuxScriptedWindow(t, "reset", "primary", backups), filepath.Join(t.TempDir(), "try-omarchy"), dir)
	if !strings.Contains(result, "start clean at its next launch") {
		t.Fatalf("reset result: %s", result)
	}
	if archives, _ := filepath.Glob(filepath.Join(backups, "try-omarchy-backup-*.zip")); len(archives) != 1 {
		t.Fatalf("backup before reset: %v", archives)
	}
	current, err := os.ReadFile(filepath.Join(dir, "vm", "disk.raw"))
	if err != nil || !bytes.HasPrefix(current, []byte("fixture guest/rootfs.ext4")) {
		t.Fatalf("clean disk was not published: %v", err)
	}
	kept := linuxRetainedResetDisks(dir)
	if len(kept) != 1 {
		t.Fatalf("kept disks: %v", kept)
	}
	if data, err := os.ReadFile(kept[0]); err != nil || string(data) != "personal files" {
		t.Fatalf("kept disk: %q %v", data, err)
	}
	result = showLinuxRecoveryInWindow(linuxScriptedWindow(t, "clean-reset", "secondary"), filepath.Join(t.TempDir(), "try-omarchy"), dir)
	if !strings.Contains(result, "was removed") || len(linuxRetainedResetDisks(dir)) != 0 {
		t.Fatalf("kept disk removal: %s", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "vm", "disk.raw")); err != nil {
		t.Fatalf("current disk removed: %v", err)
	}
}

func TestLinuxResetStopsWhenBackupIsCancelled(t *testing.T) {
	dir := resetReadyFixture(t)
	result := showLinuxRecoveryInWindow(linuxScriptedWindow(t, "reset", "primary", "cancel"), filepath.Join(t.TempDir(), "try-omarchy"), dir)
	if !strings.Contains(result, "The VM was kept") {
		t.Fatalf("cancelled backup result: %s", result)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "vm", "disk.raw")); err != nil || string(data) != "personal files" {
		t.Fatalf("disk changed after cancelled backup: %q %v", data, err)
	}
	if len(linuxRetainedResetDisks(dir)) != 0 {
		t.Fatal("reset ran after the backup was cancelled")
	}
}
