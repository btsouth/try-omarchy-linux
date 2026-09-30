//go:build linux

package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestLinuxRecoveryBurstKeepsFileTransitionsAndCompletion(t *testing.T) {
	at := time.Unix(100, 0)
	var states []linuxSetupState
	progress := linuxRecoveryProgressUpdates(func(s linuxSetupState) { states = append(states, s) }, "Backing up", func() time.Time { return at })
	for n := int64(1); n <= 100000; n++ {
		progress(n, 100000, "vm/disk.raw")
	}
	if len(states) != 2 || states[0].Current != 1 || states[1].Current != 100000 || states[1].Total != 100000 {
		t.Fatalf("chunk burst must emit initial and complete progress without flooding GTK: %+v", states)
	}
	progress(0, 0, "guest/rootfs.ext4")
	progress(1, 0, "guest/rootfs.ext4")
	if len(states) != 3 || states[2].Status != "Backing up: rootfs.ext4" {
		t.Fatalf("new file must appear immediately; unknown totals must still be throttled: %+v", states)
	}
}

func TestLinuxRecoveryProgressResumesAfterInterval(t *testing.T) {
	at := time.Unix(100, 0)
	var states []linuxSetupState
	progress := linuxRecoveryProgressUpdates(func(s linuxSetupState) { states = append(states, s) }, "Restoring", func() time.Time { return at })
	progress(1, 100, "disk.raw")
	at = at.Add(99 * time.Millisecond)
	progress(20, 100, "disk.raw")
	if len(states) != 1 {
		t.Fatalf("update arrived before render interval: %+v", states)
	}
	at = at.Add(time.Millisecond)
	progress(30, 100, "disk.raw")
	if len(states) != 2 || states[1].Current != 30 {
		t.Fatalf("latest progress was not delivered at the interval: %+v", states)
	}
	progress(100, 100, "disk.raw")
	if len(states) != 3 || states[2].Current != 100 {
		t.Fatalf("completion must not wait for the interval: %+v", states)
	}
}

func TestLinuxRecoverySpaceFailureNamesTheNextAction(t *testing.T) {
	err := fmt.Errorf("backup: %w", &insufficientSpaceError{path: "/run/user/1000/doc/opaque/backups", need: 32 << 30, have: 22 << 30})
	message := linuxRecoveryResult(err, "saved")
	for _, want := range []string{"32.0 GiB", "22.0 GiB", "Choose a folder with more space", "original VM was kept"} {
		if !strings.Contains(message, want) {
			t.Fatalf("space failure lacks %q: %s", want, message)
		}
	}
	if strings.Contains(message, "/run/user") || strings.Contains(message, "opaque") {
		t.Fatalf("internal permission-grant paths reached the normal recovery message: %s", message)
	}
}
