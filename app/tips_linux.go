//go:build linux

package main

import (
	"os"
	"path/filepath"
)

// The first time Omarchy's desktop is ready for a person, one notification says
// the things only the host can: how to get the keyboard back, how to go
// fullscreen and how to move files. It is a notification, not a window, so it
// never covers the guest's own welcome, takes no focus and goes away on its own.
// The guest already explains its menu to a trial account; a personal account has
// no such welcome, so the menu is mentioned only there. Shown once per person.

const linuxTipsShownName = "session-tips-shown"

func linuxTipsMarker() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "try-omarchy", linuxTipsShownName), nil
}

func linuxTipsShown() bool {
	path, err := linuxTipsMarker()
	if err != nil {
		return true // no place to remember it: never nag
	}
	_, err = os.Stat(path)
	return err == nil
}

func markLinuxTipsShown() {
	path, err := linuxTipsMarker()
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o700)
	}
	if err == nil {
		err = os.WriteFile(path, []byte("shown\n"), 0o600)
	}
	if err != nil {
		logf("tips: could not remember that they were shown: %v", err)
	}
}

func linuxSessionTips(trialAccount bool) (title, body string) {
	body = "Omarchy has your keyboard while its window is focused. Ctrl+Alt+G gives it back to your desktop until you click the window again. Ctrl+Alt+F switches fullscreen, and files dropped on the window go to Omarchy."
	if !trialAccount {
		body += " Super+Space opens Omarchy's menu."
	}
	return "Omarchy is ready", body
}

// showLinuxSessionTips runs once Omarchy's desktop has appeared. It is marked
// shown only after the notification was accepted, so a desktop without a
// notification service tries again next time instead of losing it.
func showLinuxSessionTips(trialAccount bool) {
	if !linuxGUIEnabled || linuxTipsShown() {
		return
	}
	title, body := linuxSessionTips(trialAccount)
	if err := linuxNotify("session-tips", title, body); err != nil {
		logf("tips: could not show the notification: %v", err)
		return
	}
	markLinuxTipsShown()
}
