//go:build linux

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestLinuxOnlyPortalDisksDisableQemuLocking(t *testing.T) {
	for _, tc := range []struct {
		path     string
		disabled bool
	}{
		{fmt.Sprintf("/run/user/%d/doc/abc123/folder/try-omarchy/vm/disk.raw", os.Getuid()), true},
		{fmt.Sprintf("/run/user/%d/doc/abc123/folder,comma/vm/disk.raw", os.Getuid()), true},
		{fmt.Sprintf("/run/user/%d/omabox/private-runtime/doc/abc123/folder/vm/disk.raw", os.Getuid()), true},
		{fmt.Sprintf("/run/user/%d/doc/../ordinary/disk.raw", os.Getuid()), false},
		{fmt.Sprintf("/run/user/%d/omabox/private-runtime/mydoc/abc123/vm/disk.raw", os.Getuid()), false},
		{"/home/me/try-omarchy/vm/disk.raw", false},
	} {
		cfg := &config{disk: tc.path, diskFormat: "raw"}
		args := []string{"-drive", "file=" + qemuOptionValue(tc.path) + ",format=raw,if=virtio", "-drive", "file=other.raw,format=raw,if=virtio"}
		got := linuxQemuArgs(cfg, args)
		if strings.Contains(got[1], "locking=off") != tc.disabled || strings.Contains(got[3], "locking=off") {
			t.Fatalf("%s: %v", tc.path, got)
		}
	}
}
