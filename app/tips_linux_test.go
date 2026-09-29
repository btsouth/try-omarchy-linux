//go:build linux

package main

import (
	"errors"
	"strings"
	"testing"
)

func tipsTest(t *testing.T) *[]string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	oldGUI, oldNotify := linuxGUIEnabled, linuxNotify
	t.Cleanup(func() { linuxGUIEnabled, linuxNotify = oldGUI, oldNotify })
	linuxGUIEnabled = true
	var bodies []string
	linuxNotify = func(id, title, body string) error {
		if id != "session-tips" || title != "Omarchy is ready" {
			t.Errorf("notification %q %q", id, title)
		}
		bodies = append(bodies, body)
		return nil
	}
	return &bodies
}

func TestSessionTipsAppearOnceAndSayWhatOnlyTheHostCanTell(t *testing.T) {
	shown := tipsTest(t)
	showLinuxSessionTips(true)
	showLinuxSessionTips(true)
	if len(*shown) != 1 {
		t.Fatalf("shown %d times, want once", len(*shown))
	}
	for _, want := range []string{"Ctrl+Alt+G gives your keyboard back", "clicking the window returns it", "Ctrl+Alt+F", "fullscreen", "dropped on the window"} {
		if !strings.Contains((*shown)[0], want) {
			t.Errorf("tip lacks %q: %s", want, (*shown)[0])
		}
	}
	if !strings.HasPrefix((*shown)[0], "Ctrl+Alt+G") {
		t.Errorf("a collapsed notification shows only its start, which must be the way back: %s", (*shown)[0])
	}
	if strings.Contains((*shown)[0], "Super+Space") {
		t.Errorf("the guest already welcomes a trial account with its menu key, so the host must not repeat it: %s", (*shown)[0])
	}
}

func TestPersonalAccountsAreToldTheMenuKeyBecauseNoGuestWelcomeDoes(t *testing.T) {
	shown := tipsTest(t)
	showLinuxSessionTips(false)
	if len(*shown) != 1 || !strings.Contains((*shown)[0], "Super+Space opens Omarchy's menu.") {
		t.Fatalf("personal account tip: %v", *shown)
	}
}

func TestTipsAreNotLostWhenTheDesktopHasNoNotificationService(t *testing.T) {
	shown := tipsTest(t)
	linuxNotify = func(id, title, body string) error { return errors.New("no notification service") }
	showLinuxSessionTips(true)
	if linuxTipsShown() {
		t.Fatal("a tip that could not be shown must not be marked as shown")
	}
	linuxNotify = func(id, title, body string) error { *shown = append(*shown, body); return nil }
	showLinuxSessionTips(true)
	if len(*shown) != 1 || !linuxTipsShown() {
		t.Fatalf("second try: %v", *shown)
	}
}

func TestNoTipsWithoutAWindowSession(t *testing.T) {
	shown := tipsTest(t)
	linuxGUIEnabled = false
	showLinuxSessionTips(true)
	if len(*shown) != 0 {
		t.Fatal("terminal-only runs must not notify")
	}
}
