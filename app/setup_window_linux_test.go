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

// This fixture speaks the real pipe protocol without loading GTK or a display.
func TestLinuxSetupHelperProcess(t *testing.T) {
	mode := os.Getenv("TRY_OMARCHY_SETUP_TEST")
	if mode == "" {
		return
	}
	fmt.Println(`{"event":"ready"}`)
	// linux-recovery-script answers each prompt with the next of these replies.
	script := strings.Split(os.Getenv("TRY_OMARCHY_RECOVERY_REPLIES"), "\n")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var state linuxSetupState
		if json.Unmarshal(scanner.Bytes(), &state) != nil {
			os.Exit(2)
		}
		// Tests that check what the window was sent read this record.
		if record := os.Getenv("TRY_OMARCHY_STATE_RECORD"); record != "" && state.Request != 0 {
			if f, err := os.OpenFile(record, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				f.Write(append(append([]byte(nil), scanner.Bytes()...), '\n'))
				f.Close()
			}
		}
		switch mode {
		case "linux-recovery-script":
			if state.Request == 0 {
				continue
			}
			value := "cancel"
			if len(script) > 0 && script[0] != "" {
				value, script = script[0], script[1:]
			}
			json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: value})
		case "linux-recovery-backup", "linux-recovery-restore":
			var value string
			switch state.Prompt {
			case "recovery":
				if mode == "linux-recovery-backup" {
					value = "backup"
				} else {
					value = "restore"
				}
			case "backup-folder", "restore-parent":
				value = os.Getenv("TRY_OMARCHY_RECOVERY_FOLDER")
			case "restore-archive":
				value = os.Getenv("TRY_OMARCHY_RECOVERY_ARCHIVE")
			default:
				continue
			}
			json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: value})
		case "prompt":
			// A stale reply must never answer the next question.
			json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request - 1, Value: "shutdown"})
			json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: state.Prompt})
		case "cancel":
			fmt.Println(`{"event":"cancel"}`)
		case "crash":
			os.Exit(2)
		case "error":
			if state.Error && state.Status == "Could not read /tmp/日本語\nTry again." {
				fmt.Println(`{"event":"dismissed"}`)
				os.Exit(0)
			}
		case "runtime-error":
			if state.Error && state.ErrorTitle == "File transfer failed" && state.Status == "Try the drop again." {
				fmt.Println(`{"event":"dismissed"}`)
				os.Exit(0)
			}
		case "settings-save", "settings-cancel", "settings-running":
			value := "cancel"
			if state.Prompt == "settings-saved" && mode == "settings-running" {
				value = "back"
			} else if mode == "settings-save" || mode == "settings-running" {
				if state.Settings == nil || state.Settings.Share != "/saved/folder" || state.Settings.ShareEnabled {
					os.Exit(3)
				}
				form := *state.Settings
				form.Memory = "4096"
				form.CPUs = "1"
				form.Render = "cpu"
				data, _ := json.Marshal(form)
				value = string(data)
			}
			json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: value})
		case "settings-profile":
			form := *state.Settings
			form.ResourceProfile = os.Getenv("TRY_OMARCHY_PROFILE")
			data, _ := json.Marshal(form)
			json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: string(data)})
		case "settings-partial-retry":
			form := *state.Settings
			blocked := filepath.Join(os.Getenv("TRY_OMARCHY_SETTINGS_DIR"), resourcePreferencesFilename)
			if strings.Contains(state.Status, "Could not save") {
				if !strings.Contains(state.Status, "Could not save resource profile") || !strings.Contains(state.Status, "Already saved: VM configuration") || form.Memory != "4096" || form.ResourceProfile != "manual" || form.Microphone {
					os.Exit(3)
				}
				if err := os.Remove(blocked); err != nil {
					os.Exit(4)
				}
			} else if err := os.Mkdir(blocked, 0o700); err != nil {
				os.Exit(5)
			}
			form.Memory, form.ResourceProfile, form.Microphone = "4096", "manual", false
			data, _ := json.Marshal(form)
			json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: string(data)})
		case "settings-share-save":
			if state.Prompt != "settings" || state.Settings == nil {
				os.Exit(3)
			}
			form := *state.Settings
			form.Share = os.Getenv("TRY_OMARCHY_SETUP_SHARE")
			form.ShareEnabled = true
			data, _ := json.Marshal(form)
			json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: string(data)})
		case "settings-clipboard-off", "settings-clipboard-on", "settings-clipboard-hidden":
			form := *state.Settings
			switch mode {
			case "settings-clipboard-off":
				if !form.ClipboardAvailable || !form.ClipboardShare {
					os.Exit(3)
				}
				form.ClipboardShare = false
			case "settings-clipboard-on":
				if !form.ClipboardAvailable || form.ClipboardShare {
					os.Exit(3)
				}
				form.ClipboardShare = true
			default:
				if form.ClipboardAvailable {
					os.Exit(3)
				}
				form.ClipboardShare = false
			}
			data, _ := json.Marshal(form)
			json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: string(data)})
		case "settings-phase9-save", "settings-phase9-reload":
			if state.Prompt != "settings" || state.Settings == nil || state.Settings.Fullscreen != (mode == "settings-phase9-reload") || state.Settings.Microphone != (mode == "settings-phase9-save") {
				os.Exit(3)
			}
			value := "cancel"
			if mode == "settings-phase9-save" {
				form := *state.Settings
				form.Fullscreen = true
				form.Microphone = false
				data, _ := json.Marshal(form)
				value = string(data)
			}
			json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: value})
		}
	}
	os.Exit(0)
}

func setupWindowFixture(t *testing.T, mode string) *linuxSetupWindow {
	t.Helper()
	configureSetupCancellation(false)
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxSetupHelperProcess$")
	cmd.Env = append(os.Environ(), "TRY_OMARCHY_SETUP_TEST="+mode)
	w := launchLinuxSetupWindow(cmd)
	if w == nil {
		t.Fatal("helper failed to start")
	}
	t.Cleanup(func() { w.stop(); configureSetupCancellation(false) })
	return w
}

func TestLinuxSetupNormalFinishDoesNotCancel(t *testing.T) {
	w := setupWindowFixture(t, "normal")
	w.update(linuxSetupState{Status: "Starting Omarchy"})
	w.stop()
	w.stop()
	if setupCancelled() {
		t.Fatal("normal handoff cancelled setup")
	}
}

func TestLinuxSetupCancelAndHelperCrash(t *testing.T) {
	for _, mode := range []string{"cancel", "crash"} {
		t.Run(mode, func(t *testing.T) {
			w := setupWindowFixture(t, mode)
			w.update(linuxSetupState{Status: "Downloading"})
			select {
			case <-setupContext().Done():
			case <-time.After(3 * time.Second):
				t.Fatal("setup kept running after helper cancellation or crash")
			}
		})
	}
}

func TestLinuxSetupErrorRoundTrip(t *testing.T) {
	w := setupWindowFixture(t, "error")
	u := newLinuxProgressUI()
	u.window = w
	done := make(chan struct{})
	go func() { u.showError("Could not read /tmp/日本語\nTry again."); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("error was lost or dismissal did not unblock launcher")
	}
	if setupCancelled() {
		t.Fatal("error dismissal was treated as cancellation")
	}
}

func TestLinuxRuntimeErrorDismissalKeepsGuestRunning(t *testing.T) {
	w := setupWindowFixture(t, "runtime-error")
	done := make(chan struct{})
	go func() {
		showLinuxRuntimeErrorOnWindow(w, "File transfer failed", "Try the drop again.")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runtime error was not displayed and dismissed")
	}
	if setupCancelled() {
		t.Fatal("dismissing a runtime error cancelled the VM")
	}
}

func TestLinuxSetupSlowReaderKeepsLatestState(t *testing.T) {
	w := &linuxSetupWindow{updates: make(chan linuxSetupState, 1)}
	for i := int64(0); i <= 10000; i++ {
		w.update(linuxSetupState{Status: "Verifying", Current: i, Total: 10000})
	}
	if state := <-w.updates; state.Current != 10000 {
		t.Fatalf("stale state: %+v", state)
	}
}

func TestLinuxPromptRejectsStaleReply(t *testing.T) {
	w := setupWindowFixture(t, "prompt")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, kind := range []string{"location", "account", "close"} {
		got, err := w.ask(ctx, linuxSetupState{Prompt: kind})
		if err != nil || got != kind {
			t.Fatalf("%s: %q %v", kind, got, err)
		}
	}
}

func TestLinuxCloseHelperCrashKeepsGuestRunning(t *testing.T) {
	configureSetupCancellation(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxSetupHelperProcess$")
	cmd.Env = append(os.Environ(), "TRY_OMARCHY_SETUP_TEST=crash")
	w := launchLinuxWindow(cmd, cancel)
	if w == nil {
		t.Fatal("helper failed to start")
	}
	defer w.stop()
	answer, err := w.ask(ctx, linuxSetupState{Prompt: "close"})
	if err == nil || answer == "shutdown" || setupCancelled() {
		t.Fatalf("crash accepted: %q %v cancel=%t", answer, err, setupCancelled())
	}
}
