//go:build linux

package main

import (
	"github.com/godbus/dbus/v5"
	"testing"
)

func TestLinuxKeyboardEnvironment(t *testing.T) {
	for _, c := range []struct{ layout, variant, want string }{{"us", "intl", "us"}, {"de", "", "de"}, {"us,de", "", ""}, {"us", "intl,", ""}, {";sh", "", ""}} {
		got, _ := linuxKeyboardEnvironment(c.layout, c.variant)
		if got != c.want {
			t.Errorf("%q %q: %q", c.layout, c.variant, got)
		}
	}
}

func TestLinuxTrayDispatch(t *testing.T) {
	before := linuxGUIEnabled
	linuxGUIEnabled = true
	defer func() { linuxGUIEnabled = before }()
	drain := func(ch chan struct{}) {
		for {
			select {
			case <-ch:
			default:
				return
			}
		}
	}
	drain(linuxSettingsRequests)
	drain(linuxShutdownRequests)
	defer drain(linuxSettingsRequests)
	defer drain(linuxShutdownRequests)
	menu := &linuxTrayMenu{}
	menu.Event(1, "hovered", dbus.MakeVariant(""), 0)
	if len(linuxSettingsRequests) != 0 {
		t.Fatal("hover opened settings")
	}
	menu.Event(1, "clicked", dbus.MakeVariant(""), 0)
	menu.Event(2, "clicked", dbus.MakeVariant(""), 0)
	if len(linuxSettingsRequests) != 1 || len(linuxShutdownRequests) != 1 {
		t.Fatal("menu actions not dispatched")
	}
	root := linuxTrayLayout(0)
	if len(root.Children) != 2 {
		t.Fatal(root)
	}
	// Pin the wire type consumed by dbusmenu hosts.
	if got := dbus.SignatureOf(root).String(); got != "(ia{sv}av)" {
		t.Fatal(got)
	}
}

func TestLinuxScaleOverride(t *testing.T) {
	for _, value := range []string{"NaN", "Inf", "0", "4.1", "garbage"} {
		if _, err := parseLinuxScale(value); err == nil {
			t.Fatal(value)
		}
	}
	before := linuxGuestScale
	defer func() { linuxGuestScale = before }()
	linuxGuestScale = "auto"
	for _, entry := range linuxQemuEnvironment([]string{"QEMU_SDL_GUEST_SCALE=4"}) {
		if entry == "QEMU_SDL_GUEST_SCALE=4" {
			t.Fatal("stale override inherited")
		}
	}
}
