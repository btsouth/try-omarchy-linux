//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxLocationPrecedenceAndMissingDrive(t *testing.T) {
	root := t.TempDir()
	standard, alternate := filepath.Join(root, "default"), filepath.Join(root, "drive", "try-omarchy")
	if err := os.MkdirAll(alternate, 0700); err != nil {
		t.Fatal(err)
	}
	calls := 0
	choose := func(string) (string, bool, error) { calls++; return alternate, true, nil }
	got, proceed, err := resolveLinuxDataDirectory(standard, standard, false, choose)
	if err != nil || !proceed || got != alternate || calls != 1 {
		t.Fatalf("initial choice: %q %t %v (%d calls)", got, proceed, err, calls)
	}
	got, _, err = resolveLinuxDataDirectory(standard, standard, false, choose)
	if err != nil || got != alternate || calls != 1 {
		t.Fatalf("saved choice: %q %v", got, err)
	}
	if err := os.RemoveAll(filepath.Dir(alternate)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveLinuxDataDirectory(standard, standard, false, choose); err == nil {
		t.Fatal("missing drive silently replaced")
	}
	override := filepath.Join(root, "explicit")
	got, _, err = resolveLinuxDataDirectory(standard, override, true, choose)
	if err != nil || got != override || calls != 1 {
		t.Fatalf("explicit override: %q %v", got, err)
	}
}

func writeLinuxAttachFixture(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"vm/disk.raw", "guest/build-spec.json", "guest/rootfs.ext4", "guest/vmlinuz-linux", "guest/initramfs-linux.img"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("retained "+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLinuxAttachExistingVMAndKeepPreviousOnFailure(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "default", "try-omarchy")
	previous := filepath.Join(root, "old-drive", "try-omarchy")
	restored := filepath.Join(root, "restored-copy")
	writeLinuxAttachFixture(t, previous)
	writeLinuxAttachFixture(t, restored)
	if err := saveDataLocationPointer(defaultDir, previous); err != nil {
		t.Fatal(err)
	}
	if _, err := attachLinuxDataDirectory(defaultDir, filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing folder accepted")
	}
	partial := filepath.Join(root, "partial")
	writeLinuxAttachFixture(t, partial)
	if err := os.Remove(filepath.Join(partial, "guest", "initramfs-linux.img")); err != nil {
		t.Fatal(err)
	}
	if _, err := attachLinuxDataDirectory(defaultDir, partial); err == nil {
		t.Fatal("incomplete VM accepted")
	}
	if selected, found, err := loadDataLocationPointer(defaultDir); err != nil || !found || selected != previous {
		t.Fatalf("failed attach changed pointer: %q %t %v", selected, found, err)
	}
	if got, err := attachLinuxDataDirectory(defaultDir, restored); err != nil || got != restored {
		t.Fatalf("reattach restored copy: %q %v", got, err)
	}
	if selected, found, err := loadDataLocationPointer(defaultDir); err != nil || !found || selected != restored {
		t.Fatalf("restored pointer: %q %t %v", selected, found, err)
	}
	for _, dir := range []string{previous, restored} {
		if _, err := os.Stat(filepath.Join(dir, "vm", "disk.raw")); err != nil {
			t.Fatalf("VM changed at %s: %v", dir, err)
		}
	}
	writeLinuxAttachFixture(t, defaultDir)
	if _, err := attachLinuxDataDirectory(defaultDir, defaultDir); err != nil {
		t.Fatal(err)
	}
	if _, found, err := loadDataLocationPointer(defaultDir); err != nil || found {
		t.Fatalf("default reattach did not clear pointer: %t %v", found, err)
	}
}

func TestLinuxForgetUnavailableLocationKeepsVMAndSharedFiles(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "default", "try-omarchy")
	selected := filepath.Join(root, "removable", "try-omarchy")
	shared := filepath.Join(root, "shared")
	writeLinuxAttachFixture(t, selected)
	if err := os.MkdirAll(shared, 0700); err != nil {
		t.Fatal(err)
	}
	sharedFile := filepath.Join(shared, "keep.txt")
	if err := os.WriteFile(sharedFile, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := saveDataLocationPointer(defaultDir, selected); err != nil {
		t.Fatal(err)
	}
	missing := selected + "-offline"
	if err := os.Rename(selected, missing); err != nil {
		t.Fatal(err)
	}
	state, dir := linuxHomeState(defaultDir)
	if dir != "" || !state.CanForget || state.Path != selected {
		t.Fatalf("missing location home: %+v %q", state, dir)
	}
	if _, found, err := loadDataLocationPointer(defaultDir); err != nil || !found {
		t.Fatalf("pointer changed before confirmation: %t %v", found, err)
	}
	if err := forgetLinuxDataLocation(defaultDir); err != nil {
		t.Fatal(err)
	}
	if _, found, err := loadDataLocationPointer(defaultDir); err != nil || found {
		t.Fatalf("pointer remained after confirmation: %t %v", found, err)
	}
	if _, err := os.Stat(filepath.Join(missing, "vm", "disk.raw")); err != nil {
		t.Fatalf("VM removed: %v", err)
	}
	if _, err := os.Stat(sharedFile); err != nil {
		t.Fatalf("shared file removed: %v", err)
	}
	if err := os.Rename(missing, selected); err != nil {
		t.Fatal(err)
	}
	if _, err := attachLinuxDataDirectory(defaultDir, selected); err != nil {
		t.Fatalf("reattach after reconnect: %v", err)
	}
	if pointer, found, err := loadDataLocationPointer(defaultDir); err != nil || !found || pointer != selected {
		t.Fatalf("reattached pointer: %q %t %v", pointer, found, err)
	}
}

func TestLinuxForgetCorruptPointerButRejectSymlink(t *testing.T) {
	defaultDir := filepath.Join(t.TempDir(), "try-omarchy")
	if err := os.MkdirAll(defaultDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := dataLocationPointerPath(defaultDir)
	if err := os.WriteFile(path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	state, dir := linuxHomeState(defaultDir)
	if dir != "" || !state.CanForget {
		t.Fatalf("corrupt pointer home: %+v %q", state, dir)
	}
	if err := forgetLinuxDataLocation(defaultDir); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if err := forgetLinuxDataLocation(defaultDir); err == nil {
		t.Fatal("symlink pointer accepted")
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "keep" {
		t.Fatalf("outside file changed: %q %v", data, err)
	}
}

func TestLinuxDataLocationSelectableAfterPrebootSettings(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "default", "try-omarchy")
	if err := os.MkdirAll(defaultDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath(defaultDir), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if selectable, err := linuxDataLocationSelectable(defaultDir, defaultDir); err != nil || !selectable {
		t.Fatalf("preboot settings rejected: selectable=%t err=%v", selectable, err)
	}
	if selectable, err := linuxDataLocationSelectable(defaultDir, filepath.Join(root, "other")); err != nil || selectable {
		t.Fatalf("unrelated settings-only folder accepted: selectable=%t err=%v", selectable, err)
	}
	if err := os.WriteFile(filepath.Join(defaultDir, "unrelated.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if selectable, err := linuxDataLocationSelectable(defaultDir, defaultDir); err != nil || selectable {
		t.Fatalf("occupied folder accepted: selectable=%t err=%v", selectable, err)
	}
}

func TestLinuxProvisionChoices(t *testing.T) {
	for _, tc := range []struct {
		name, saved, answer            string
		disk, fresh, explicit, instant bool
		want                           string
		calls                          int
	}{
		{name: "new trial", answer: "instant", want: "instant", calls: 1},
		{name: "new personal", answer: "personal", want: "personal", calls: 1},
		{name: "resume", saved: "instant", want: "instant"},
		{name: "fresh asks again", saved: "instant", disk: true, fresh: true, answer: "personal", want: "personal", calls: 1},
		{name: "explicit false", saved: "instant", explicit: true, want: "personal"},
		{name: "explicit true", explicit: true, instant: true, want: "instant"},
		{name: "old disk", disk: true, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := &config{dir: dir, disk: filepath.Join(dir, "disk.raw"), fresh: tc.fresh, instant: tc.instant}
			if tc.saved != "" {
				if err := writeProvisionMode(dir, tc.saved); err != nil {
					t.Fatal(err)
				}
			}
			if tc.disk {
				if err := os.WriteFile(cfg.disk, []byte("existing disk"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			err := chooseLinuxProvisionMode(cfg, tc.explicit, func() (string, error) { calls++; return tc.answer, nil })
			if err != nil {
				t.Fatal(err)
			}
			mode, _ := readProvisionMode(dir)
			if mode != tc.want || calls != tc.calls || cfg.instant != (tc.want == "instant") {
				t.Fatalf("mode=%q instant=%t calls=%d", mode, cfg.instant, calls)
			}
			if tc.disk {
				data, _ := os.ReadFile(cfg.disk)
				if string(data) != "existing disk" {
					t.Fatal("disk modified by account chooser")
				}
			}
		})
	}
}

func TestLinuxProvisionCancelledDoesNotSave(t *testing.T) {
	cfg := &config{dir: t.TempDir()}
	cfg.disk = filepath.Join(cfg.dir, "disk.raw")
	err := chooseLinuxProvisionMode(cfg, false, func() (string, error) { return "", errSetupCancelled })
	if !errors.Is(err, errSetupCancelled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.dir, provisionModeFilename)); !os.IsNotExist(err) {
		t.Fatal("cancelled choice was persisted")
	}
}

func TestLinuxResourceProfileFollowsFirstStorageChoice(t *testing.T) {
	root := t.TempDir()
	original, selected := filepath.Join(root, "default"), filepath.Join(root, "custom")
	if err := saveSettings(settingsPath(original), settings{MemoryMiB: 2048, CPUs: 1}); err != nil {
		t.Fatal(err)
	}
	if err := saveResourcePreferences(original, resourceBalanced); err != nil {
		t.Fatal(err)
	}
	got, proceed, err := resolveLinuxDataDirectory(original, original, false, func(string) (string, bool, error) { return selected, true, nil })
	if err != nil || !proceed || got != selected {
		t.Fatalf("storage: %q %v %v", got, proceed, err)
	}
	profile, err := loadResourcePreferences(selected)
	if err != nil || profile.Profile != resourceBalanced {
		t.Fatalf("profile lost: %+v %v", profile, err)
	}
}
