//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxTransferSourceDocumentGrant(t *testing.T) {
	base := t.TempDir()
	runtime := filepath.Join(base, "runtime")
	root := filepath.Join(base, "flatpak", "doc")
	granted := filepath.Join(root, "grant-1", "café 世界.txt")
	if err := os.MkdirAll(filepath.Dir(granted), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(granted, []byte("portal contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "flatpak", "doc"), filepath.Join(runtime, "doc")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	portalPath := filepath.Join(runtime, "doc", "grant-1", "café 世界.txt")
	got, err := linuxTransferSource(portalPath)
	if err != nil || got != granted {
		t.Fatalf("document grant path: %q, %v", got, err)
	}
	if archive, err := packClipboardFiles([]string{got}); err != nil || len(archive) == 0 {
		t.Fatalf("document grant did not enter the file-transfer archive: %v", err)
	}
	paths, err := linuxGrantDroppedFiles([]string{portalPath})
	if err != nil || len(paths) != 1 || paths[0] != granted {
		t.Fatalf("document drop path: %q, %v", paths, err)
	}
	escape := filepath.Join(base, "outside.txt")
	if err := os.WriteFile(escape, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(escape, filepath.Join(root, "grant-1", "escape.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := linuxTransferSource(filepath.Join(runtime, "doc", "grant-1", "escape.txt")); err == nil || !strings.Contains(err.Error(), "outside the document grant") {
		t.Fatalf("document grant escape accepted: %v", err)
	}
	otherLink := filepath.Join(base, "ordinary-link.txt")
	if err := os.Symlink(escape, otherLink); err != nil {
		t.Fatal(err)
	}
	if _, err := linuxTransferSource(otherLink); err == nil || !strings.Contains(err.Error(), "linked paths") {
		t.Fatalf("ordinary symlink accepted: %v", err)
	}
}
