//go:build linux

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

type testBackgroundRequest struct {
	parent  string
	options map[string]dbus.Variant
}

type testBackgroundPortal struct {
	conn       *dbus.Conn
	requests   chan testBackgroundRequest
	code       uint32
	result     any
	noResponse bool
}

func (p *testBackgroundPortal) RequestBackground(sender dbus.Sender, parent string, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	p.requests <- testBackgroundRequest{parent, options}
	path := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + strings.ReplaceAll(strings.TrimPrefix(string(sender), ":"), ".", "_") + "/" + options["handle_token"].Value().(string))
	result := p.result
	if result == nil {
		result = options["autostart"].Value()
	}
	if !p.noResponse {
		if err := p.conn.Emit(path, "org.freedesktop.portal.Request.Response", p.code, map[string]dbus.Variant{"autostart": dbus.MakeVariant(result)}); err != nil {
			return "", dbus.MakeFailedError(err)
		}
	}
	return path, nil
}

func backgroundPortalFixture(t *testing.T, code uint32, result any, noResponse bool) *testBackgroundPortal {
	t.Helper()
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", privatePowerBus(t))
	server, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	if reply, err := server.RequestName(linuxPortalDesktop, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("portal name: %v %v", reply, err)
	}
	portal := &testBackgroundPortal{conn: server, requests: make(chan testBackgroundRequest, 8), code: code, result: result, noResponse: noResponse}
	if err := server.Export(portal, linuxPortalObject, "org.freedesktop.portal.Background"); err != nil {
		t.Fatal(err)
	}
	return portal
}

func TestLinuxSignInPortalChoices(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enable", false: "disable"}[enabled], func(t *testing.T) {
			portal := backgroundPortalFixture(t, 0, nil, false)
			previous := launchPreferences{StartAutomatically: true, LaunchAtSignIn: !enabled}
			dir := t.TempDir()
			if err := saveLaunchPreferences(dir, previous); err != nil {
				t.Fatal(err)
			}
			next := previous
			next.LaunchAtSignIn = enabled
			if err := saveLinuxLaunchPreferences(context.Background(), dir, previous, next); err != nil {
				t.Fatal(err)
			}
			request := <-portal.requests
			if request.parent != "" || request.options["autostart"].Value() != enabled || request.options["reason"].Value() != "Open Try Omarchy when you log in." {
				t.Fatalf("bad portal request: %+v", request)
			}
			if !reflect.DeepEqual(request.options["commandline"].Value(), []string{"try-omarchy", "-autostart"}) {
				t.Fatalf("bad command: %v", request.options["commandline"])
			}
			if linuxDirectStart(map[string]bool{"autostart": true}) {
				t.Fatal("login skips the home")
			}
			got, err := loadLaunchPreferences(dir)
			if err != nil || got.LaunchAtSignIn != enabled || !got.StartAutomatically {
				t.Fatalf("preferences: %+v %v", got, err)
			}
		})
	}
}

func TestLinuxSignInPortalDenialKeepsPreferences(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    uint32
		result  any
		enabled bool
	}{
		{"request declined", 1, nil, true},
		{"autostart declined", 0, false, true},
		{"disable declined", 0, true, false},
		{"invalid result", 0, "yes", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backgroundPortalFixture(t, tc.code, tc.result, false)
			dir := t.TempDir()
			previous := launchPreferences{StartAutomatically: true, LaunchAtSignIn: !tc.enabled}
			if err := saveLaunchPreferences(dir, previous); err != nil {
				t.Fatal(err)
			}
			next := previous
			next.LaunchAtSignIn = tc.enabled
			err := saveLinuxLaunchPreferences(context.Background(), dir, previous, next)
			if err == nil || !strings.Contains(err.Error(), "did not allow") {
				t.Fatalf("denial: %v", err)
			}
			got, err := loadLaunchPreferences(dir)
			if err != nil || got.LaunchAtSignIn != previous.LaunchAtSignIn || !got.StartAutomatically {
				t.Fatalf("denial changed preferences: %+v %v", got, err)
			}
		})
	}
}

func TestLinuxSignInPortalAbsentAndTimeout(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		t.Setenv("DBUS_SESSION_BUS_ADDRESS", privatePowerBus(t))
		dir := t.TempDir()
		previous := launchPreferences{}
		if err := saveLinuxLaunchPreferences(context.Background(), dir, previous, launchPreferences{LaunchAtSignIn: true}); err == nil || !strings.Contains(err.Error(), "permission service") {
			t.Fatalf("missing portal: %v", err)
		}
		if prefs, err := loadLaunchPreferences(dir); err != nil || prefs.LaunchAtSignIn {
			t.Fatalf("missing portal saved enable: %+v %v", prefs, err)
		}
		// Automatic VM startup remains usable when no portal is installed.
		if err := saveLinuxLaunchPreferences(context.Background(), dir, previous, launchPreferences{StartAutomatically: true}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		backgroundPortalFixture(t, 0, nil, true)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if err := requestLinuxSignIn(ctx, true); err == nil || !strings.Contains(err.Error(), "saved choice has not changed") {
			t.Fatalf("timeout: %v", err)
		}
	})
}

func TestLinuxSignInSaveFailureRestoresPortal(t *testing.T) {
	portal := backgroundPortalFixture(t, 0, nil, false)
	dir := t.TempDir()
	// A directory at the preferences path prevents the final atomic rename.
	if err := os.Mkdir(filepath.Join(dir, launchPreferencesFilename), 0700); err != nil {
		t.Fatal(err)
	}
	if err := saveLinuxLaunchPreferences(context.Background(), dir, launchPreferences{}, launchPreferences{LaunchAtSignIn: true}); err == nil {
		t.Fatal("write failure ignored")
	}
	for _, enabled := range []bool{true, false} {
		if request := <-portal.requests; request.options["autostart"].Value() != enabled {
			t.Fatalf("restore request: %+v", request)
		}
	}
}

func TestLinuxSignInSettingsFlow(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved", true: "denied"}[denied], func(t *testing.T) {
			code := uint32(0)
			if denied {
				code = 1
			}
			portal := backgroundPortalFixture(t, code, nil, false)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			dir := t.TempDir()
			previous := launchPreferences{StartAutomatically: true, LaunchAtSignIn: denied}
			if err := saveLaunchPreferences(dir, previous); err != nil {
				t.Fatal(err)
			}
			record := filepath.Join(t.TempDir(), "states.jsonl")
			t.Setenv("TRY_OMARCHY_STATE_RECORD", record)
			reply, _ := json.Marshal(map[string]bool{"launchAtSignIn": !denied})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			message := showLinuxSettingsInWindow(ctx, linuxScriptedWindow(t, string(reply), "cancel"), dir, false)
			if !denied && !strings.Contains(message, "Settings saved") {
				t.Fatalf("save: %q", message)
			}
			select {
			case request := <-portal.requests:
				if request.options["autostart"].Value() != !denied {
					t.Fatalf("toggle: %+v", request)
				}
			default:
				t.Fatal("Settings did not call portal")
			}
			got, err := loadLaunchPreferences(dir)
			if err != nil || !got.StartAutomatically || !got.LaunchAtSignIn {
				t.Fatalf("settings preferences: %+v %v", got, err)
			}
			data, err := os.ReadFile(record)
			if err != nil {
				t.Fatal(err)
			}
			var state linuxSetupState
			if err := json.Unmarshal([]byte(strings.SplitN(string(data), "\n", 2)[0]), &state); err != nil {
				t.Fatal(err)
			}
			if state.Settings.LaunchAtSignIn != previous.LaunchAtSignIn {
				t.Fatal("Settings did not load the saved toggle")
			}
			if denied && !strings.Contains(string(data), "did not allow this login change") {
				t.Fatalf("denial not shown: %s", data)
			}
		})
	}
}
