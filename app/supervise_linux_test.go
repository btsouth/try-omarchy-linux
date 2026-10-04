//go:build linux

package main

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLinuxForceStopWaitsForShutdownGrace(t *testing.T) {
	answer := make(chan bool, 1)
	confirmation := newLinuxShutdownConfirmation(func(context.Context) <-chan bool { return answer })
	defer confirmation.close()
	forcePrompts := 0
	confirmation.openForce = func(context.Context) <-chan bool {
		forcePrompts++
		return answer
	}
	confirmation.requestShutdown(0, time.Now())
	if confirmation.pending != answer || confirmation.force {
		t.Fatal("first shutdown did not use the normal confirmation")
	}
	confirmation.pending = nil
	confirmation.shutdownAt = time.Now()
	confirmation.requestShutdown(1, confirmation.shutdownAt.Add(linuxShutdownGracePeriod-time.Nanosecond))
	if forcePrompts != 0 || confirmation.pending != nil {
		t.Fatal("force stop was offered before the grace period")
	}
	confirmation.requestShutdown(1, confirmation.shutdownAt.Add(linuxShutdownGracePeriod))
	confirmation.requestShutdown(1, confirmation.shutdownAt.Add(linuxShutdownGracePeriod))
	if forcePrompts != 1 || confirmation.pending != answer || !confirmation.force {
		t.Fatal("force stop was missing or duplicated after the grace period")
	}
	state := linuxForceStopState()
	if !state.Destructive || state.Secondary != "Force stop" || !strings.Contains(state.Status, "Unsaved work") {
		t.Fatalf("force stop warning: %+v", state)
	}
}

func TestLinuxWindowAndTrayCanConfirmForceStop(t *testing.T) {
	oldGUI, oldGrace := linuxGUIEnabled, linuxShutdownGracePeriod
	linuxGUIEnabled, linuxShutdownGracePeriod = true, 0
	t.Cleanup(func() { linuxGUIEnabled, linuxShutdownGracePeriod = oldGUI, oldGrace })
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	for _, source := range []string{"window", "tray"} {
		t.Run(source, func(t *testing.T) {
			answer := make(chan bool)
			prompted := make(chan bool, 3)
			confirmation := newLinuxShutdownConfirmation(func(context.Context) <-chan bool {
				prompted <- false
				return answer
			})
			confirmation.openForce = func(context.Context) <-chan bool {
				prompted <- true
				return answer
			}
			defer confirmation.close()
			qmp, peer := testQMPStream(t)
			commands := make(chan string, 3)
			go func() {
				scanner := bufio.NewScanner(peer)
				for scanner.Scan() {
					commands <- scanner.Text()
				}
			}()
			lines := make(chan string)
			exited := make(chan error, 1)
			done := make(chan struct{})
			go func() {
				watchLinux(&config{}, qmp, &exec.Cmd{}, exited, make(chan os.Signal), lines,
					&linuxVisibility{}, 0, confirmation, false)
				close(done)
			}()
			defer func() {
				exited <- nil
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Error("supervisor did not exit")
				}
			}()
			request := func(force bool) {
				if source == "window" {
					lines <- `{"event":"DISPLAY_CLOSE_REQUEST"}`
				} else {
					linuxShutdownRequests <- struct{}{}
				}
				select {
				case got := <-prompted:
					if got != force {
						t.Fatalf("force prompt=%t, want %t", got, force)
					}
				case <-time.After(time.Second):
					t.Fatal("shutdown did not open confirmation")
				}
				select {
				case command := <-commands:
					t.Fatalf("unconfirmed shutdown sent %s", command)
				default:
				}
			}
			command := func(want string) {
				select {
				case got := <-commands:
					if !strings.Contains(got, want) {
						t.Fatalf("QMP command=%s, want %s", got, want)
					}
				case <-time.After(time.Second):
					t.Fatal("confirmed shutdown did not reach QEMU")
				}
			}
			request(false)
			answer <- true
			command("system_powerdown")
			request(true)
			answer <- false // Keep waiting must leave QEMU running.
			request(true)
			answer <- true
			command("quit")
		})
	}
}

func TestLinuxWatchReportsOnlyUnexpectedQEMUFailure(t *testing.T) {
	oldGUI, oldUI := linuxGUIEnabled, linuxUI
	linuxGUIEnabled, linuxUI = false, newLinuxProgressUI()
	t.Cleanup(func() { linuxGUIEnabled, linuxUI = oldGUI, oldUI })
	failure := exec.Command("sh", "-c", "exit 23").Run()
	if failure == nil {
		t.Fatal("expected process failure")
	}
	for _, tc := range []struct {
		name      string
		exit      error
		requests  int
		wantError bool
	}{
		{"crash", failure, 0, true},
		{"clean poweroff", nil, 0, false},
		{"requested shutdown", failure, 1, false},
		{"force stop", failure, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			exited := make(chan error, 1)
			exited <- tc.exit
			confirmation := newLinuxShutdownConfirmation(nil)
			defer confirmation.close()
			err := watchLinux(&config{vmDir: dir}, nil, &exec.Cmd{}, exited, nil, nil,
				&linuxVisibility{}, tc.requests, confirmation, false)
			if (err != nil) != tc.wantError {
				t.Fatalf("supervisor failure=%v, want error=%t", err, tc.wantError)
			}
			if err != nil && (!errors.Is(err, failure) || !strings.Contains(err.Error(), filepath.Join(dir, "qemu-stderr.log"))) {
				t.Fatalf("failure lost process error or log path: %v", err)
			}
		})
	}
}

func TestLinuxQEMUFailureShowsIndependentDialog(t *testing.T) {
	oldGUI, oldShow := linuxGUIEnabled, showLinuxRuntimeErrorWindow
	linuxGUIEnabled = true
	t.Cleanup(func() { linuxGUIEnabled, showLinuxRuntimeErrorWindow = oldGUI, oldShow })
	calls := 0
	showLinuxRuntimeErrorWindow = func(title, detail string) {
		calls++
		if title != "Omarchy stopped unexpectedly" || detail != "runtime failure" {
			t.Fatalf("runtime dialog: %q %q", title, detail)
		}
	}
	reportLinuxQEMUFailure(errors.New("runtime failure"))
	if calls != 1 {
		t.Fatalf("runtime dialog calls=%d", calls)
	}
	linuxGUIEnabled = false
	reportLinuxQEMUFailure(errors.New("terminal failure"))
	if calls != 1 {
		t.Fatal("terminal mode opened a dialog")
	}
}
