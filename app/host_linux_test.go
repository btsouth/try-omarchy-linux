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
	if got := windowedKernelCmdline(spec); got != "root=/dev/vda console=ttyS0 quiet vt.global_cursor_default=0" {
		t.Fatalf("got %q", got)
	}
}
