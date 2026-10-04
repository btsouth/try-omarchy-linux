//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func consentTest(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
}

func TestClipboardConsentExplainsBeforeGNOMEAsksAndRemembersASkip(t *testing.T) {
	consentTest(t)
	var asked []linuxSetupState
	answer := "primary"
	ask := func(s linuxSetupState) (string, error) { asked = append(asked, s); return answer, nil }
	if !linuxClipboardConsent(ask) || len(asked) != 1 {
		t.Fatalf("Continue must go on to GNOME's own dialog after one explanation: %d", len(asked))
	}
	// Continuing stores nothing: GNOME's token, once granted, is the memory.
	if linuxClipboardSharingOff() {
		t.Fatal("continuing must not record a refusal")
	}
	answer = "secondary"
	if linuxClipboardConsent(ask) {
		t.Fatal("Not now must not open GNOME's dialog")
	}
	if !linuxClipboardSharingOff() {
		t.Fatal("Not now must be remembered so the next launch does not ask again")
	}
	asked = nil
	if linuxClipboardConsent(ask) || len(asked) != 0 {
		t.Fatalf("a person who declined must not be asked again: asked %d times", len(asked))
	}
}

func TestClipboardConsentLeavesWhoAlreadyGrantedAccessAlone(t *testing.T) {
	consentTest(t)
	if err := saveLinuxClipboardRestoreToken("granted-earlier"); err != nil {
		t.Fatal(err)
	}
	ask := func(linuxSetupState) (string, error) {
		t.Fatal("explained again to someone GNOME already remembers")
		return "", nil
	}
	if !linuxClipboardConsent(ask) {
		t.Fatal("a preview user with a saved permission must keep clipboard sharing")
	}
}

func TestClipboardConsentWithoutAWindowKeepsTheTerminalBehavior(t *testing.T) {
	consentTest(t)
	if !linuxClipboardConsent(nil) {
		t.Fatal("no window to ask in: request access as before")
	}
}

func TestClipboardConsentClosedOrCancelledIsNotADecision(t *testing.T) {
	consentTest(t)
	configureSetupCancellation(false)
	t.Cleanup(func() { configureSetupCancellation(false) })
	if linuxClipboardConsent(func(linuxSetupState) (string, error) { return "cancel", nil }) || linuxClipboardSharingOff() {
		t.Fatal("dismissing the explanation skips this launch only")
	}
	if !setupCancelled() {
		t.Fatal("Cancel must stop setup, not silently continue to boot")
	}
	configureSetupCancellation(false)
	if linuxClipboardConsent(func(linuxSetupState) (string, error) { return "", context.Canceled }) || linuxClipboardSharingOff() {
		t.Fatal("a closed window skips this launch only")
	}
}

func TestClipboardDenialInGNOMEIsRememberedButATimeoutIsNot(t *testing.T) {
	consentTest(t)
	rememberLinuxClipboardDenial(context.DeadlineExceeded)
	rememberLinuxClipboardDenial(errors.New("portal connection closed"))
	if linuxClipboardSharingOff() {
		t.Fatal("only the person's own refusal may stop the question")
	}
	rememberLinuxClipboardDenial(fmt.Errorf("%w (the request was declined)", errLinuxClipboardDenied))
	if !linuxClipboardSharingOff() {
		t.Fatal("a refusal in GNOME's dialog must be remembered")
	}
	if err := setLinuxClipboardSharing(true); err != nil || linuxClipboardSharingOff() {
		t.Fatalf("turning sharing back on must forget the refusal: %v", err)
	}
	dir, _ := os.UserConfigDir()
	if _, err := os.Stat(filepath.Join(dir, "try-omarchy", linuxClipboardChoiceName)); !os.IsNotExist(err) {
		t.Fatalf("the refusal file should be gone: %v", err)
	}
}

func TestClipboardExplanationSaysWhatGNOMEWillShowAndWhatIsNotShared(t *testing.T) {
	s := linuxClipboardConsentState()
	all := s.Status
	for _, row := range s.Sections[0].Rows {
		all += "\n" + row.Title + "\n" + row.Detail
	}
	for _, want := range []string{"Remote Desktop", "Allow Clipboard Access", "Allow Remote Interaction", "press Share", "never moves your pointer or types for you", "still drop files", "Settings"} {
		if !strings.Contains(all, want) {
			t.Errorf("explanation lacks %q:\n%s", want, all)
		}
	}
	if s.Primary != "Continue" || s.Secondary != "Not now" || s.Destructive {
		t.Fatalf("buttons: %+v", s)
	}
}

func settingsClipboardRun(t *testing.T, mode string) {
	t.Helper()
	w := setupWindowFixture(t, mode)
	defer w.stop()
	if got := showLinuxSettingsInWindow(context.Background(), w, filepath.Join(t.TempDir(), "try-omarchy"), false); got == "" {
		t.Fatalf("%s: settings were not saved", mode)
	}
}

func TestSettingsCanTurnClipboardSharingOffAndBackOnOnEveryDesktop(t *testing.T) {
	for _, desktop := range []struct{ name, wayland, display string }{
		{"GNOME", "wayland-test", ""},
		{"KDE", "wayland-test", ""},
		{"Hyprland", "wayland-test", ""},
		{"Xfce", "", ":test"},
	} {
		t.Run(desktop.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("WAYLAND_DISPLAY", desktop.wayland)
			t.Setenv("DISPLAY", desktop.display)
			t.Setenv("XDG_CURRENT_DESKTOP", desktop.name)
			settingsClipboardRun(t, "settings-clipboard-off")
			if !linuxClipboardSharingOff() {
				t.Fatal("unchecking the setting must record the choice")
			}
			settingsClipboardRun(t, "settings-clipboard-on")
			if linuxClipboardSharingOff() {
				t.Fatal("checking the setting must allow clipboard sharing again")
			}
		})
	}
}

func TestSettingsHideClipboardSharingWithoutAGraphicalSession(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	settingsClipboardRun(t, "settings-clipboard-hidden")
	if linuxClipboardSharingOff() {
		t.Fatal("a hidden setting must not record a refusal")
	}
}
