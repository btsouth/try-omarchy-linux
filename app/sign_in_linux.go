//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

type linuxSignInError string

func (e linuxSignInError) Error() string { return string(e) }

// The portal wraps this command in flatpak run. Follow the saved data-location
// pointer at login, and use the normal home and StartAutomatically countdown.
func linuxSignInCommandline() []string {
	return []string{"try-omarchy", "-autostart"}
}

func requestLinuxSignIn(parent context.Context, enabled bool) error {
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		logf("start with login: %v", err)
		return linuxSignInError("Start with login is unavailable on this desktop. Your saved choice has not changed.")
	}
	defer conn.Close()
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	if err = conn.AddMatchSignal(dbus.WithMatchSender(linuxPortalDesktop), dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")); err != nil {
		logf("start with login: %v", err)
		return linuxSignInError("Start with login is unavailable on this desktop. Your saved choice has not changed.")
	}
	results, err := linuxPortalRequest(ctx, conn, signals, "org.freedesktop.portal.Background.RequestBackground", "", map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(fmt.Sprintf("trylogin_%d", time.Now().UnixNano())),
		"autostart":    dbus.MakeVariant(enabled),
		"commandline":  dbus.MakeVariant(linuxSignInCommandline()),
		"reason":       dbus.MakeVariant("Open Try Omarchy when you log in."),
	})
	if err != nil {
		logf("start with login: %v", err)
		if errors.Is(err, errLinuxClipboardDenied) {
			return linuxSignInError("Your desktop did not allow this login change. Your saved choice has not changed.")
		}
		return linuxSignInError("Could not change Start with login. Your saved choice has not changed. Try again when the desktop permission service is available.")
	}
	allowed, valid := results["autostart"].Value().(bool)
	if !valid || allowed != enabled {
		return linuxSignInError("Your desktop did not allow this login change. Your saved choice has not changed.")
	}
	return nil
}

// Save only the confirmed portal choice. Restore it if the preferences cannot
// be saved, just as Windows restores its Startup shortcut after a write error.
func saveLinuxLaunchPreferences(ctx context.Context, dir string, previous, next launchPreferences) error {
	changed := next.LaunchAtSignIn != previous.LaunchAtSignIn
	if changed {
		if err := requestLinuxSignIn(ctx, next.LaunchAtSignIn); err != nil {
			return err
		}
	}
	if err := saveLaunchPreferences(dir, next); err != nil {
		if changed {
			rollback, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if rollbackErr := requestLinuxSignIn(rollback, previous.LaunchAtSignIn); rollbackErr != nil {
				logf("restore start with login: %v", rollbackErr)
				return linuxSignInError(fmt.Sprintf("Could not save startup preferences or restore the login choice. Check Start with login in your desktop settings: %v", err))
			}
		}
		return err
	}
	return nil
}
