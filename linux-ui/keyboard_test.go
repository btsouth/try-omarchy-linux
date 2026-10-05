package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestKeyboardHelpReportsUnknownOrDetectedHost(t *testing.T) {
	for _, tc := range []struct{ host, want string }{
		{"", "Host layout could not be detected. Choose a layout explicitly, or keep the guest choice."},
		{"de:nodeadkeys", "Detected host layout: de:nodeadkeys."},
	} {
		var form settingsForm
		data := `{"hostKeyboard":` + strconv.Quote(tc.host) + `}`
		if err := json.Unmarshal([]byte(data), &form); err != nil {
			t.Fatal(err)
		}
		got := keyboardHelpText(form.HostKeyboard)
		if !strings.Contains(got, tc.want) || !strings.Contains(got, "Ctrl+Alt+G") {
			t.Fatalf("%q: %q", tc.host, got)
		}
	}
}

func TestSettingsSubmissionLeavesHostDetectionWithLauncher(t *testing.T) {
	data, err := json.Marshal(settingsForm{Keyboard: "us"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "hostKeyboard") {
		t.Fatal("a submission would erase the launcher's detected host layout")
	}
}
