//go:build windows

package main

import (
	"strings"
	"testing"
)

// The Windows launcher depends on these: upstream WHPX wedges on an in-guest
// reset, and any XSAVE/AVX feature panics the guest kernel in CPU mode.
func TestWindowsQemuArgsKeepWHPXWorkarounds(t *testing.T) {
	for _, gpu := range []bool{true, false} {
		cfg := &config{vmDir: `C:\vm`, guestDir: `C:\guest`, disk: `C:\vm\disk.raw`, diskFormat: "raw", memMiB: 4096, audio: "sdl", useGpu: gpu}
		args := strings.Join(buildQemuArgs(cfg, "root=/dev/vda"), " ")
		if !strings.Contains(args, "-no-reboot") {
			t.Fatalf("gpu=%v: -no-reboot missing: %s", gpu, args)
		}
		if !strings.Contains(args, "accel=whpx") {
			t.Fatalf("gpu=%v: WHPX accelerator missing: %s", gpu, args)
		}
	}
	cfg := &config{vmDir: `C:\vm`, guestDir: `C:\guest`, disk: `C:\vm\disk.raw`, diskFormat: "raw", memMiB: 4096, audio: "sdl"}
	if args := strings.Join(buildQemuArgs(cfg, "root=/dev/vda"), " "); !strings.Contains(args, "-cpu qemu64,+ssse3,+sse4.1,+sse4.2,+popcnt,+aes ") {
		t.Fatalf("CPU rendering flags changed: %s", args)
	}
}
