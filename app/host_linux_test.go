//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProcStatCPUCountsIdleAndIowaitAsNotBusy(t *testing.T) {
	busy, total, ok := parseProcStatCPU(strings.NewReader("cpu  100 5 50 800 40 3 2 0 7 0\ncpu0 1 2 3 4 5 6 7 8 9 10\n"))
	if !ok {
		t.Fatal("aggregate cpu line not parsed")
	}
	if total != 1000 || busy != 160 {
		t.Fatalf("busy=%d total=%d, want 160 of 1000", busy, total)
	}
	if _, _, ok := parseProcStatCPU(strings.NewReader("intr 1 2 3\n")); ok {
		t.Fatal("a non-cpu first line was accepted")
	}
}

func TestParseMeminfoReadsTotalAndAvailable(t *testing.T) {
	total, available := parseMeminfo(strings.NewReader("MemTotal:       32768000 kB\nMemFree:  1000 kB\nMemAvailable:   16384000 kB\n"))
	if total != 32000 || available != 16000 {
		t.Fatalf("total=%d available=%d MiB", total, available)
	}
}

func TestZoneFromLocaltimeLink(t *testing.T) {
	for target, want := range map[string]string{
		"/usr/share/zoneinfo/America/New_York":       "America/New_York",
		"../usr/share/zoneinfo/Europe/Berlin":        "Europe/Berlin",
		"/usr/share/zoneinfo/posix/Asia/Tokyo":       "Asia/Tokyo",
		"/var/db/timezone/zoneinfo/Australia/Sydney": "Australia/Sydney",
		"/etc/somewhere-else":                        "",
	} {
		if got := zoneFromLocaltimeLink(target); got != want {
			t.Errorf("%s -> %q, want %q", target, got, want)
		}
	}
}

func TestPosixLocaleNameSkipsCAndStripsEncoding(t *testing.T) {
	if got := posixLocaleName("", "C.UTF-8", "de_DE.UTF-8"); got != "de_DE" {
		t.Fatalf("got %q, want de_DE", got)
	}
	if got := posixLocaleName("sr_RS@latin"); got != "sr_RS" {
		t.Fatalf("got %q, want sr_RS", got)
	}
	if got := posixLocaleName("POSIX", ""); got != "" {
		t.Fatalf("got %q, want no preference", got)
	}
}

func TestHostLocaleKeepAndOverrides(t *testing.T) {
	zone, layout, variant, locale := hostLocale("keep", "us:intl", "keep")
	if zone != "" || locale != "" || layout != "us" || variant != "intl" {
		t.Fatalf("got zone=%q layout=%q variant=%q locale=%q", zone, layout, variant, locale)
	}
	zone, _, _, locale = hostLocale("Europe/Paris", "", "fr_FR")
	if zone != "Europe/Paris" || locale != "fr_FR" {
		t.Fatalf("overrides not used: zone=%q locale=%q", zone, locale)
	}
}

func TestValidateLinuxSharedFolder(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	share := filepath.Join(root, "Work")
	for _, dir := range []string{data, share, filepath.Join(data, "inner")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := validateLinuxSharedFolder(share, data); err != nil || got != share {
		t.Fatalf("plain folder: got %q, %v", got, err)
	}
	if _, err := validateLinuxSharedFolder(root, data); err == nil {
		t.Fatal("a folder containing the data folder was accepted")
	}
	if _, err := validateLinuxSharedFolder(filepath.Join(data, "inner"), data); err == nil {
		t.Fatal("a folder inside the data folder was accepted")
	}
	if _, err := validateLinuxSharedFolder(filepath.Join(root, "missing"), data); err == nil {
		t.Fatal("a missing folder was accepted")
	}
}

func TestLinuxQemuArgsUseKVMAndResetInPlace(t *testing.T) {
	for _, gpu := range []bool{true, false} {
		cfg := &config{vmDir: "/vm", guestDir: "/guest", disk: "/vm/disk.raw", diskFormat: "raw", memMiB: 4096, audio: "pipewire", useGpu: gpu}
		args := strings.Join(buildQemuArgs(cfg, "root=/dev/vda"), " ")
		if strings.Contains(args, "-no-reboot") {
			t.Fatalf("gpu=%v: KVM guests reboot in place: %s", gpu, args)
		}
		if !strings.Contains(args, "-machine q35,accel=kvm -cpu host ") {
			t.Fatalf("gpu=%v: KVM machine and host CPU missing: %s", gpu, args)
		}
		if !strings.Contains(args, "-monitor none -parallel none") || !strings.Contains(args, "virtio-balloon-pci,free-page-reporting=on") {
			t.Fatalf("gpu=%v: Linux-only devices missing: %s", gpu, args)
		}
		if !strings.Contains(args, "-audiodev pipewire,id=snd") {
			t.Fatalf("gpu=%v: audio backend missing: %s", gpu, args)
		}
	}
}

func TestWindowedKernelCmdline(t *testing.T) {
	var spec buildSpec
	spec.Runtime.KernelCommandLine = "root=/dev/vda console=tty0 console=hvc0 quiet"
	if got := windowedKernelCmdline(spec); got != "root=/dev/vda console=ttyS0 quiet vt.global_cursor_default=0 edd=off" {
		t.Fatalf("got %q", got)
	}
}

func TestCloseRequestedMatchesOnlyTheRuntimeEvent(t *testing.T) {
	if !closeRequested(`{"timestamp": {"seconds": 1, "microseconds": 2}, "event": "DISPLAY_CLOSE_REQUEST", "data": {"display": 0}}`) {
		t.Fatal("close request not recognised")
	}
	if closeRequested(`{"event": "SHUTDOWN", "data": {"guest": true, "reason": "guest-shutdown"}}`) || closeRequested(`{"return": {}}`) {
		t.Fatal("another message matched")
	}
}

func TestLinuxQemuEnvironmentTurnsOnDesktopBehavior(t *testing.T) {
	env := strings.Join(linuxQemuEnvironment(nil), " ")
	for _, want := range []string{"SDL_VIDEO_WAYLAND_WMCLASS=" + linuxAppID, "QEMU_SDL_TITLE_FROM_NAME=1", "QEMU_SDL_FOCUS_KEYBOARD_GRAB=1"} {
		if !strings.Contains(env, want) {
			t.Fatalf("%s missing from %s", want, env)
		}
	}
}

func TestOnlyNVIDIARenderNodes(t *testing.T) {
	root := t.TempDir()
	add := func(node, vendor string) {
		dir := filepath.Join(root, node, "device")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "vendor"), []byte(vendor+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if onlyNVIDIARenderNodes(renderNodeVendors(root)) {
		t.Fatal("no GPUs counted as NVIDIA")
	}
	add("renderD128", "0x10de")
	if !onlyNVIDIARenderNodes(renderNodeVendors(root)) {
		t.Fatal("a single NVIDIA GPU was not recognised")
	}
	add("renderD129", "0x8086")
	if onlyNVIDIARenderNodes(renderNodeVendors(root)) {
		t.Fatal("a hybrid Intel and NVIDIA laptop counted as NVIDIA only")
	}
}

func TestVulkanPresentMode(t *testing.T) {
	for _, c := range []struct {
		flag   string
		nvidia bool
		want   string
	}{{"auto", true, "cpu"}, {"auto", false, "gpu"}, {"gpu", true, "gpu"}, {"cpu", false, "cpu"}} {
		if got, err := vulkanPresentMode(c.flag, c.nvidia); err != nil || got != c.want {
			t.Errorf("%s nvidia=%v: got %q %v, want %q", c.flag, c.nvidia, got, err, c.want)
		}
	}
	if _, err := vulkanPresentMode("fast", false); err == nil {
		t.Fatal("an unknown mode was accepted")
	}
}
