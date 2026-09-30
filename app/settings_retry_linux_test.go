//go:build linux

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This helper speaks the setup pipe protocol without loading GTK. It replaces
// the preference file with a directory for the first Save, then restores it.
func TestLinuxMicrophoneRetryHelper(t *testing.T) {
	dir := os.Getenv("TRY_OMARCHY_MIC_RETRY_DIR")
	if dir == "" {
		return
	}
	path := filepath.Join(dir, desktopPreferencesFilename)
	wantDisabled := os.Getenv("TRY_OMARCHY_MIC_RETRY_DISABLED") == "true"
	fail := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	fmt.Println(`{"event":"ready"}`)
	scanner := bufio.NewScanner(os.Stdin)
	count := 0
	for scanner.Scan() {
		var state linuxSetupState
		fail(json.Unmarshal(scanner.Bytes(), &state))
		if state.Prompt != "settings" {
			continue
		}
		count++
		switch count {
		case 1:
			fail(os.Rename(path, path+".backup"))
			fail(os.Mkdir(path, 0o700))
		case 2:
			if !strings.Contains(state.Status, "Could not save camera and microphone access") || !strings.Contains(state.Status, "Already saved: VM configuration") {
				panic("first Save error was not shown")
			}
			fail(os.WriteFile(filepath.Join(dir, "first-save-error"), []byte(state.Status), 0o600))
			fail(os.Remove(path))
			fail(os.Rename(path+".backup", path))
		default:
			panic("Save did not finish after retry")
		}
		form := *state.Settings
		if os.Getenv("TRY_OMARCHY_CAMERA_RETRY") == "1" {
			form.Camera = !wantDisabled
		} else {
			form.Microphone = !wantDisabled
		}
		value, err := json.Marshal(form)
		fail(err)
		fail(json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: string(value)}))
	}
}

func TestLinuxCameraSaveRetryPersistsChoice(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		dir := t.TempDir()
		before := desktopPreferences{CameraDisabled: !disabled, MicrophoneDisabled: true, CameraID: "saved-camera", AutomaticUpdatesDisabled: true}
		if err := saveDesktopPreferences(dir, before); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxMicrophoneRetryHelper$")
		cmd.Env = append(os.Environ(), "TRY_OMARCHY_MIC_RETRY_DIR="+dir, "TRY_OMARCHY_CAMERA_RETRY=1", fmt.Sprintf("TRY_OMARCHY_MIC_RETRY_DISABLED=%t", disabled))
		w := launchLinuxWindow(cmd, func() {})
		if w == nil {
			t.Fatal("retry helper did not start")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		feedback := showLinuxSettingsInWindow(ctx, w, dir, false)
		cancel()
		w.stop()
		if !strings.Contains(feedback, "Settings saved") {
			t.Fatalf("camera save: %s", feedback)
		}
		after, err := loadDesktopPreferences(dir)
		if err != nil || after.CameraDisabled != disabled || after.MicrophoneDisabled != before.MicrophoneDisabled || after.CameraID != before.CameraID || after.AutomaticUpdatesDisabled != before.AutomaticUpdatesDisabled {
			t.Fatalf("camera retry changed preferences: %+v %v", after, err)
		}
	}
}

func TestLinuxMicrophoneSaveRetryPersistsChoice(t *testing.T) {
	for _, wantDisabled := range []bool{false, true} {
		name := "enable"
		if wantDisabled {
			name = "disable"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			before := desktopPreferences{MicrophoneDisabled: !wantDisabled, CameraDisabled: true, CameraID: "saved-camera", AutomaticUpdatesDisabled: true}
			if err := saveDesktopPreferences(dir, before); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxMicrophoneRetryHelper$")
			cmd.Env = append(os.Environ(), "TRY_OMARCHY_MIC_RETRY_DIR="+dir, fmt.Sprintf("TRY_OMARCHY_MIC_RETRY_DISABLED=%t", wantDisabled))
			w := launchLinuxWindow(cmd, func() {})
			if w == nil {
				t.Fatal("retry helper did not start")
			}
			defer w.stop()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			feedback := showLinuxSettingsInWindow(ctx, w, dir, false)
			if !strings.Contains(feedback, "Settings saved") {
				t.Fatalf("retry did not finish: %q", feedback)
			}
			if _, err := os.ReadFile(filepath.Join(dir, "first-save-error")); err != nil {
				t.Fatalf("first Save failure was not observed: %v", err)
			}
			after, err := loadDesktopPreferences(dir)
			if err != nil {
				t.Fatal(err)
			}
			if after.MicrophoneDisabled != wantDisabled || after.CameraDisabled != before.CameraDisabled || after.CameraID != before.CameraID || after.AutomaticUpdatesDisabled != before.AutomaticUpdatesDisabled {
				t.Fatalf("retry persisted wrong preferences: got %+v, previous %+v", after, before)
			}
		})
	}
}
