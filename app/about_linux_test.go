//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestVersionLabelsReadLikeAPersonWritesThem(t *testing.T) {
	for in, want := range map[string]string{"v0.1.0": "0.1.0", "v0.1.0-preview.2": "0.1.0 preview 2", "0.2.0-rc.1": "0.2.0 rc.1"} {
		if got := linuxVersionLabel(in); got != want {
			t.Errorf("linuxVersionLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHelpAnswersTheFirstHourQuestions(t *testing.T) {
	state := linuxAboutState()
	all := state.Status
	headings := map[string]bool{}
	for _, section := range state.Sections {
		headings[section.Heading] = true
		for _, row := range section.Rows {
			all += "\n" + row.Title + "\n" + row.Detail
		}
	}
	for _, want := range []string{"Keyboard, mouse and window", "Files", "Removing the app and your VM", "Updates", "This version"} {
		if !headings[want] {
			t.Errorf("help lacks the %q section", want)
		}
	}
	for _, want := range []string{
		"Ctrl+Alt+G", "Ctrl+Alt+F", "Super+Space", "Super+K",
		"App Settings & Data", "--delete-data", "never touched", "Use existing data folder",
		"Software or flatpak update", "goes back to the old ones",
		"github.com/btsouth/try-omarchy-linux", "0.1.0 preview 3",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("help lacks %q", want)
		}
	}
	// The four things the uninstall page must keep apart.
	for _, want := range []string{"Uninstalling the app", "Deleting a VM", "Backups are ordinary .zip files", "delete them yourself"} {
		if !strings.Contains(all, want) {
			t.Errorf("the removal help does not distinguish: missing %q", want)
		}
	}
	if !strings.Contains(state.Status, "0.1.0 preview 3") || state.Prompt != "about" {
		t.Fatalf("about state: %+v", state)
	}
}

func TestHelpUsesOnlyPlainWords(t *testing.T) {
	state := linuxAboutState()
	text := state.Status
	for _, section := range state.Sections {
		for _, row := range section.Rows {
			text += " " + row.Title + " " + row.Detail
		}
	}
	for _, jargon := range []string{"QEMU", "virgl", "Flatpak sandbox", "document portal", "KVM", "OSTree", "—", "–"} {
		if strings.Contains(text, jargon) {
			t.Errorf("help contains %q", jargon)
		}
	}
}
