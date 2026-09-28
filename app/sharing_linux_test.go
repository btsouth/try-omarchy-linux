//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxSharingChoices(t *testing.T) {
	root := t.TempDir()
	data, share := filepath.Join(root, "data"), filepath.Join(root, "exchange")
	os.Mkdir(data, 0700)
	os.Mkdir(share, 0700)
	cfg := &config{dir: data}
	saved := settings{MemoryMiB: 4096}
	if err := configureLinuxSharing(cfg, &saved, false, false, func(string) (string, error) { return share, nil }); err != nil {
		t.Fatal(err)
	}
	got, err := loadSettings(settingsPath(data))
	if err != nil || got.Share != share || got.MemoryMiB != 4096 || !got.SharedFolderPrompted || cfg.share != share {
		t.Fatalf("%+v %v", got, err)
	}
	unexpected := func(string) (string, error) { t.Fatal("unexpected prompt"); return "", nil }
	if err := configureLinuxSharing(cfg, &saved, false, false, unexpected); err != nil {
		t.Fatal(err)
	}
	if err := configureLinuxSharing(cfg, &saved, true, true, unexpected); err != nil {
		t.Fatal(err)
	}
	if err := configureLinuxSharing(cfg, &saved, false, true, func(string) (string, error) { return "", errors.New("cancel") }); err == nil || cfg.share != share {
		t.Fatal("cancel changed share")
	}
	if err := configureLinuxSharing(cfg, &saved, false, true, func(string) (string, error) { return "skip", nil }); err != nil || cfg.share != "" {
		t.Fatal("skip did not remove share", err)
	}
}

func TestLinuxSharingRejectsAliasedData(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	os.Mkdir(data, 0700)
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(data, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{data, alias, root} {
		if _, err := validateLinuxSharedFolder(path, data); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}

func TestLinuxSharingBeforeDataDirectoryExists(t *testing.T) {
	root := t.TempDir()
	realParent := filepath.Join(root, "real")
	alias := filepath.Join(root, "alias")
	share := filepath.Join(root, "exchange")
	for _, dir := range []string{realParent, share} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(realParent, alias); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		filepath.Join(root, "new-install", "try-omarchy"),
		filepath.Join(alias, "new-install", "try-omarchy"),
	} {
		if got, err := validateLinuxSharedFolder(share, data); err != nil || got != share {
			t.Fatalf("fresh data %s: share=%q err=%v", data, got, err)
		}
	}
	data := filepath.Join(alias, "new-install", "try-omarchy")
	for _, unsafe := range []string{root, realParent, alias} {
		if _, err := validateLinuxSharedFolder(unsafe, data); err == nil {
			t.Fatalf("accepted ancestor %s of aliased future data", unsafe)
		}
	}
	if err := os.MkdirAll(filepath.Join(realParent, "new-install", "try-omarchy", "inner"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := validateLinuxSharedFolder(filepath.Join(realParent, "new-install", "try-omarchy", "inner"), data); err == nil {
		t.Fatal("accepted descendant of aliased data")
	}
	if err := os.Symlink(filepath.Join(root, "missing-target"), filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	if _, err := validateLinuxSharedFolder(share, filepath.Join(root, "dangling", "new")); err == nil {
		t.Fatal("accepted unresolved data-location alias")
	}
}
