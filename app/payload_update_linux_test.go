//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLinuxGuestFixture(t *testing.T, dir, release, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rootfs.ext4"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := testSHA256([]byte(content + " manifest"))
	if err := writeInstallReceipt(dir, release, manifest, []string{"rootfs.ext4"}, map[string]string{"rootfs.ext4": testSHA256([]byte(content))}); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestLinuxUnconfirmedGuestUpdateRestoresPreviousImage(t *testing.T) {
	previous := linuxGUIEnabled
	linuxGUIEnabled = false
	t.Cleanup(func() { linuxGUIEnabled = previous })
	root := t.TempDir()
	oldManifest := writeLinuxGuestFixture(t, filepath.Join(root, "guest.previous"), "http://127.0.0.1:18090", "old image")
	writeLinuxGuestFixture(t, filepath.Join(root, "guest"), "https://example.test/linux-v0.1.0", "new image")
	if err := recordPayloadUpdate(root, "v0.1.0", true, false); err != nil {
		t.Fatal(err)
	}
	release, pin := "https://example.test/linux-v0.1.0", testSHA256([]byte("new image manifest"))
	if err := recoverLinuxGuestUpdate(root, &release, &pin); err != nil {
		t.Fatal(err)
	}
	if release != "http://127.0.0.1:18090" || pin != oldManifest {
		t.Fatalf("recovery launch did not pin the restored image: %s %s", release, pin)
	}
	if data, err := os.ReadFile(filepath.Join(root, "guest", "rootfs.ext4")); err != nil || string(data) != "old image" {
		t.Fatalf("previous image was not restored: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, payloadUpdateStateFilename)); !os.IsNotExist(err) {
		t.Fatalf("update state survived recovery: %v", err)
	}
	// A clean start after recovery leaves the pinned release alone.
	if err := recoverLinuxGuestUpdate(root, &release, &pin); err != nil || release != "http://127.0.0.1:18090" {
		t.Fatalf("second start changed the release: %s %v", release, err)
	}
}

func TestLinuxConfirmedGuestUpdateDropsPreviousImage(t *testing.T) {
	root := t.TempDir()
	writeLinuxGuestFixture(t, filepath.Join(root, "guest.previous"), "http://127.0.0.1:18090", "old image")
	newManifest := writeLinuxGuestFixture(t, filepath.Join(root, "guest"), "https://example.test/linux-v0.1.0", "new image")
	if err := recordPayloadUpdate(root, "v0.1.0", true, false); err != nil {
		t.Fatal(err)
	}
	commitGuestPayloadUpdate(root)
	if _, err := os.Stat(filepath.Join(root, "guest.previous")); !os.IsNotExist(err) {
		t.Fatalf("previous image kept after confirmation: %v", err)
	}
	release, pin := "https://example.test/linux-v0.1.0", newManifest
	if err := recoverLinuxGuestUpdate(root, &release, &pin); err != nil || release != "https://example.test/linux-v0.1.0" || pin != newManifest {
		t.Fatalf("confirmed update was rolled back: %s %s %v", release, pin, err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "guest", "rootfs.ext4")); err != nil || string(data) != "new image" {
		t.Fatalf("confirmed image changed: %q %v", data, err)
	}
}
