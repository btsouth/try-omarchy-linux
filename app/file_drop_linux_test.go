//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxFileDropRequiresGrantForExternalPath(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	file := filepath.Join(t.TempDir(), "file-é.txt")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	paths, err := linuxGrantDroppedFiles([]string{file})
	if err != nil || len(paths) != 1 || paths[0] != file {
		t.Fatalf("accessible drop changed: %v, %v", paths, err)
	}
	missing := filepath.Join(t.TempDir(), "outside-sandbox.txt")
	_, err = linuxGrantDroppedFiles([]string{missing})
	if err == nil || !strings.Contains(err.Error(), "grant access") {
		t.Fatalf("ungranted path accepted: %v", err)
	}
}
