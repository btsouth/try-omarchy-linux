//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxVirglDesktopError(t *testing.T) {
	dir := t.TempDir()
	if linuxVirglDesktopError(dir) {
		t.Fatal("missing log reported a graphics error")
	}
	path := filepath.Join(dir, "qemu-stderr.log")
	for _, tt := range []struct {
		name, log string
		want      bool
	}{
		{"mesa warning", "MESA-EGL: warning: failed to get driver name for fd -1\n", false},
		{"generic guest warning", "guest desktop announced ready\n", false},
		{"virgl command failure", "virtio_gpu_virgl_process_cmd: ctrl 0x106, error 0x1200\n", true},
		{"resource failure", "check_copy_transfer3d_handles: context error reported 9 \"quickshell\" Illegal resource 76\n", true},
		{"command buffer failure", "vrend_decode_ctx_submit_cmd: context error reported 9 \"quickshell\" Illegal command buffer 917549\n", true},
		{"old failure outside tail", "virtio_gpu_virgl_process_cmd: ctrl 0x106, error 0x1200\n" + strings.Repeat("normal output\n", 12000), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tt.log), 0600); err != nil {
				t.Fatal(err)
			}
			if got := linuxVirglDesktopError(dir); got != tt.want {
				t.Fatalf("graphics error = %v, want %v", got, tt.want)
			}
		})
	}
}
