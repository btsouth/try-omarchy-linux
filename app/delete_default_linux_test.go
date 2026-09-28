//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxDeleteDefaultVMKeepsSharedAndPreferences(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "app", "try-omarchy")
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(shared, 0700); err != nil {
		t.Fatal(err)
	}
	sharedFile := filepath.Join(shared, "personal.txt")
	if err := os.WriteFile(sharedFile, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	writeLinuxAttachFixture(t, defaultDir)
	if err := os.WriteFile(filepath.Join(defaultDir, "provision-mode"), []byte("personal"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := saveSettings(settingsPath(defaultDir), settings{Share: shared}); err != nil {
		t.Fatal(err)
	}
	if !linuxDefaultVMCanDelete(defaultDir) {
		t.Fatal("default VM was not offered for deletion")
	}
	if err := deleteLinuxDefaultVM(defaultDir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"vm", "guest", "provision-mode"} {
		if _, err := os.Stat(filepath.Join(defaultDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s remains: %v", name, err)
		}
	}
	if data, err := os.ReadFile(sharedFile); err != nil || string(data) != "keep" {
		t.Fatalf("shared file changed: %q %v", data, err)
	}
	if prefs, err := loadSettings(settingsPath(defaultDir)); err != nil || prefs.Share != shared {
		t.Fatalf("preferences changed: %+v %v", prefs, err)
	}
	if selectable, err := linuxDataLocationSelectable(defaultDir, defaultDir); err != nil || !selectable {
		t.Fatalf("default folder cannot be reused: %t %v", selectable, err)
	}
}

func TestLinuxDeleteDefaultVMRejectsUnknownAndExternalData(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "app", "try-omarchy")
	external := filepath.Join(root, "external", "try-omarchy")
	writeLinuxAttachFixture(t, defaultDir)
	writeLinuxAttachFixture(t, external)
	unknown := filepath.Join(defaultDir, "guest", "personal.txt")
	if err := os.WriteFile(unknown, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := deleteLinuxDefaultVM(defaultDir); err == nil {
		t.Fatal("unknown guest file was deleted")
	}
	if _, err := os.Stat(filepath.Join(defaultDir, "vm", "disk.raw")); err != nil {
		t.Fatalf("failed preflight deleted disk: %v", err)
	}
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
	}
	if err := saveDataLocationPointer(defaultDir, external); err != nil {
		t.Fatal(err)
	}
	if linuxDefaultVMCanDelete(defaultDir) || deleteLinuxDefaultVM(defaultDir) == nil {
		t.Fatal("external selection allowed default deletion")
	}
	for _, dir := range []string{defaultDir, external} {
		if _, err := os.Stat(filepath.Join(dir, "vm", "disk.raw")); err != nil {
			t.Fatalf("disk changed in %s: %v", dir, err)
		}
	}
}

func TestLinuxDeleteDefaultVMRejectsSymlinkAndOverlappingShare(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "app", "try-omarchy")
	writeLinuxAttachFixture(t, defaultDir)
	shared := filepath.Join(defaultDir, "vm", "shared")
	if err := saveSettings(settingsPath(defaultDir), settings{Share: shared}); err != nil {
		t.Fatal(err)
	}
	if err := deleteLinuxDefaultVM(defaultDir); err == nil {
		t.Fatal("overlapping share accepted")
	}
	if err := saveSettings(settingsPath(defaultDir), settings{}); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(defaultDir, "guest", "rootfs.ext4")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := deleteLinuxDefaultVM(defaultDir); err == nil {
		t.Fatal("linked guest file accepted")
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "keep" {
		t.Fatalf("outside file changed: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(defaultDir, "vm", "disk.raw")); err != nil {
		t.Fatalf("failed preflight deleted disk: %v", err)
	}
}
