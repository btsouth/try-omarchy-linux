//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxGuestDataPermissions(t *testing.T) {
	dir := t.TempDir()
	files := []string{"vm/disk.raw", "vm/disk.raw.part", "vm/disk.raw.incomplete-1", "vm/before-reset-old/disk.raw", "checkpoints/snapshot/vm.zip", ".snapshot-rollback-" + strings.Repeat("a", 32) + "/data/vm/disk.raw"}
	for _, name := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("guest data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "shared")
	if err := os.WriteFile(outside, []byte("shared"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "vm", "link")); err != nil {
		t.Fatal(err)
	}
	tightenLinuxGuestData(dir)
	for _, name := range files {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("private file %s: %v %v", name, info, err)
		}
		info, err = os.Stat(filepath.Dir(path))
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("private folder %s: %v %v", name, info, err)
		}
	}
	info, err := os.Stat(outside)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("linked shared file changed: %v %v", info, err)
	}
	tightenLinuxGuestData(filepath.Join(dir, "missing"))
}

func TestLinuxHomeTightensSavedGuestDataBeforeLaunch(t *testing.T) {
	stubHomeChecks(t, nil, 100<<30)
	root := t.TempDir()
	defaultDir, selected := filepath.Join(root, "default"), filepath.Join(root, "selected")
	vm := filepath.Join(selected, "vm")
	if err := os.MkdirAll(vm, 0o755); err != nil {
		t.Fatal(err)
	}
	disk := filepath.Join(vm, "disk.raw")
	if err := os.WriteFile(disk, []byte("guest data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := saveDataLocationPointer(defaultDir, selected); err != nil {
		t.Fatal(err)
	}
	if runLinuxHome(linuxScriptedWindow(t, "close"), defaultDir, "", false) {
		t.Fatal("closing the home launched the VM")
	}
	for path, mode := range map[string]os.FileMode{vm: 0o700, disk: 0o600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("startup permissions for %s: %v %v", path, info, err)
		}
	}
}
