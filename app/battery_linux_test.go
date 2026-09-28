//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxBatterySnapshot(t *testing.T) {
	root := t.TempDir()
	write := func(device, name, value string) {
		t.Helper()
		dir := filepath.Join(root, device)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("AC", "type", "Mains\n")
	write("AC", "online", "0\n")
	write("BAT0", "type", "Battery\n")
	write("BAT0", "present", "1\n")
	write("BAT0", "capacity", "57\n")
	write("BAT0", "status", "Discharging\n")
	write("BAT0", "time_to_empty_now", "8100\n")
	got := linuxBatterySnapshot(root)
	if !got.Present || got.Percentage == nil || *got.Percentage != 57 || got.State != "discharging" || got.ACConnected || got.TimeToEmptySeconds == nil || *got.TimeToEmptySeconds != 8100 {
		t.Fatalf("battery snapshot: %+v", got)
	}
	write("AC", "online", "1\n")
	write("BAT0", "status", "Charging\n")
	write("BAT0", "time_to_full_now", "1200\n")
	got = linuxBatterySnapshot(root)
	if !got.ACConnected || got.State != "charging" || got.TimeToFullSeconds == nil || *got.TimeToFullSeconds != 1200 || got.TimeToEmptySeconds != nil {
		t.Fatalf("charging snapshot: %+v", got)
	}
	write("BAT0", "present", "0\n")
	got = linuxBatterySnapshot(root)
	if got.Present || got.Percentage != nil || got.State != "unknown" {
		t.Fatalf("absent battery: %+v", got)
	}
}
