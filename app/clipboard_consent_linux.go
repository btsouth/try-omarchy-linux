//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// GNOME reaches its clipboard through the Remote Desktop portal, so its dialog
// says "Remote Desktop" and "Allow Remote Interaction". Try Omarchy uses it for
// the clipboard alone, and says so before GNOME asks. The decision lives beside
// the portal's restore token in the app's own config folder.
//
// Only "off" is ever stored: someone who declined, in the explainer or in
// GNOME's own dialog, is not asked again until they turn sharing back on in
// Settings. Anyone else is asked once, and never again once GNOME remembers.

const linuxClipboardChoiceName = "clipboard-sharing"

func linuxClipboardChoicePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "try-omarchy", linuxClipboardChoiceName), nil
}

// linuxClipboardSharingOff reports that clipboard sharing was declined.
func linuxClipboardSharingOff() bool {
	path, err := linuxClipboardChoicePath()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && len(data) < 64 && strings.TrimSpace(string(data)) == "off"
}

func setLinuxClipboardSharing(on bool) error {
	path, err := linuxClipboardChoicePath()
	if err != nil {
		return err
	}
	if on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	staged := path + ".part"
	if err := os.WriteFile(staged, []byte("off\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(staged, path)
}

func linuxClipboardConsentState() linuxSetupState {
	return linuxSetupState{Prompt: "choice", Title: "Share your clipboard with Omarchy?", Primary: "Continue", Secondary: "Not now",
		Status: "Copy and paste between your desktop and Omarchy needs GNOME's permission. GNOME asks in a window called Remote Desktop.",
		Sections: []linuxSection{{Rows: []linuxRow{
			{Title: "In GNOME's window", Detail: "Turn on Allow Clipboard Access. GNOME only enables Share once Allow Remote Interaction is on too. Then press Share."},
			{Title: "What Try Omarchy does with it", Detail: "It shares the clipboard and nothing else. It never moves your pointer or types for you."},
			{Title: "If you skip this", Detail: "Omarchy works the same, and you can still drop files on its window. Turn clipboard sharing on later in Settings."},
		}}}}
}

// linuxClipboardConsent decides whether to ask GNOME for clipboard access,
// explaining first when this person has not been asked before. Preview users who
// already granted access are not asked again.
func linuxClipboardConsent(ask func(linuxSetupState) (string, error)) bool {
	if linuxClipboardSharingOff() {
		return false
	}
	if linuxClipboardRestoreToken() != "" || ask == nil {
		return true
	}
	answer, err := ask(linuxClipboardConsentState())
	if err != nil {
		return false
	}
	if answer != "primary" {
		if answer == "secondary" {
			if err := setLinuxClipboardSharing(false); err != nil {
				logf("clipboard: could not remember the choice: %v", err)
			}
		}
		return false
	}
	return true
}

// A denial is the person's answer, not a fault. Remember it so the next launch
// does not ask again, and point at where to change their mind.
func rememberLinuxClipboardDenial(err error) {
	if !errors.Is(err, errLinuxClipboardDenied) {
		return
	}
	if e := setLinuxClipboardSharing(false); e != nil {
		logf("clipboard: could not remember the decision: %v", e)
	}
}

const linuxClipboardHowToTurnOn = "Omarchy works normally and you can still drop files on its window. To turn it on, open Try Omarchy, choose Settings, and check Share the clipboard with Omarchy."

const linuxClipboardOffMessage = "Clipboard sharing is off. " + linuxClipboardHowToTurnOn
