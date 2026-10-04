package main

import (
	"reflect"
	"strings"
	"testing"
)

// TestEscapeNeverStopsProgressOrAbandonsSetup pins what Escape answers for
// each prompt, including that progress screens ignore it.
func TestEscapeNeverStopsProgressOrAbandonsSetup(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state state
		want  string
	}{
		{"home closes", state{Prompt: "home"}, "close"},
		{"settings cancels", state{Prompt: "settings"}, "cancel"},
		{"about goes back", state{Prompt: "about"}, "back"},
		{"recovery goes back", state{Prompt: "recovery"}, "back"},
		{"shutdown prompt keeps running", state{Prompt: "close"}, "keep"},
		{"delete prompt keeps the VM", state{Prompt: "delete-default"}, "keep"},
		{"forget prompt keeps the location", state{Prompt: "forget-location"}, "keep"},
		{"a retryable error closes", state{Prompt: "error", CanRetry: true}, "close"},
		{"choice cancels", state{Prompt: "choice"}, "cancel"},
		{"a message goes back", state{Prompt: "message"}, "back"},
		{"returning reclaimed space", state{Status: "Giving unused space back...", NonCancellable: true}, ""},
		// Progress and first-run questions would cancel setup or shut Omarchy
		// down, so a stray Escape must not answer them.
		{"download progress", state{Status: "Downloading Omarchy"}, ""},
		{"booting", state{Booting: true}, ""},
		{"location question", state{Prompt: "location"}, ""},
		{"account question", state{Prompt: "account"}, ""},
		{"a move that cannot be cancelled", state{Prompt: "home", NonCancellable: true}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapeReply(tc.state); got != tc.want {
				t.Fatalf("escapeReply(%+v) = %q, want %q", tc.state, got, tc.want)
			}
		})
	}
}

func TestErrorPagesOfferTryAgainOnlyWhenRetryingCanHelp(t *testing.T) {
	if got := errorActions(state{CanRetry: true}); !reflect.DeepEqual(got, []action{
		{Label: "Try again", Reply: "retry", Suggested: true}, {Label: "Close", Reply: "close"}}) {
		t.Fatalf("retryable: %+v", got)
	}
	if got := errorActions(state{}); !reflect.DeepEqual(got, []action{{Label: "Close", Reply: "close"}}) {
		t.Fatalf("not retryable: %+v", got)
	}
}

func TestProgressPercentIsClampedAndHiddenWithoutATotal(t *testing.T) {
	for _, tc := range []struct {
		current, total int64
		want           string
	}{{0, 0, ""}, {5, -1, ""}, {0, 100, "0%"}, {42, 100, "42%"}, {150, 100, "100%"}, {-3, 100, "0%"}, {1, 3, "33%"}} {
		if got := percentText(tc.current, tc.total); got != tc.want {
			t.Errorf("percentText(%d, %d) = %q, want %q", tc.current, tc.total, got, tc.want)
		}
	}
}

func TestHomeFocusStartsOnTheSuggestedAction(t *testing.T) {
	if got := homeSuggested([]action{{Label: "Settings"}, {Label: "Launch", Suggested: true}, {Label: "Close"}}); got != 1 {
		t.Fatalf("focus index %d", got)
	}
	if got := homeSuggested([]action{{Label: "A"}, {Label: "B"}}); got != 0 {
		t.Fatalf("no suggestion should focus the first action, got %d", got)
	}
	if got := homeSuggested(nil); got != 0 {
		t.Fatalf("empty: %d", got)
	}
	if got := homeSuggested([]action{{Label: "Delete snapshot...", Reply: "delete", Destructive: true}, {Label: "Back", Reply: "close"}}); got != 1 {
		t.Fatalf("a damaged snapshot page focused a destructive action first: %d", got)
	}
}

func TestSnapshotPagesUseLauncherButtonsAndSafeEscape(t *testing.T) {
	for prompt, want := range map[string]string{"snapshots": "close", "snapshot": "close", "migration": "close", "snapshot-name": "cancel"} {
		if got := escapeReply(state{Prompt: prompt}); got != want {
			t.Errorf("%s: Escape replies %q, want %q", prompt, got, want)
		}
	}
	for prompt, want := range map[string]bool{"home": true, "error": true, "snapshots": true, "snapshot": true, "migration": true, "snapshot-name": false, "recovery": false} {
		if got := dynamicActionsPrompt(prompt); got != want {
			t.Errorf("%s: launcher buttons %v, want %v", prompt, got, want)
		}
	}
}

func TestSectionsFlattenForTheTerminalFallback(t *testing.T) {
	text := sectionsText([]section{{Heading: "Storage", Rows: []row{{Title: "Location", Detail: "~/Omarchy"}}}, {Rows: []row{{Title: "Free", Detail: "9 GB"}}}})
	for _, want := range []string{"Storage\n", "Location: ~/Omarchy\n", "Free: 9 GB\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}

func TestSettingsTimingNamesWhatAppliesOnSave(t *testing.T) {
	if got := settingsFooterText(false, false, false); got != "VM settings apply on the next launch." {
		t.Fatalf("stopped VM: %q", got)
	}
	if got := settingsFooterText(true, true, true); !strings.HasSuffix(got, "Audio devices and port forwards apply when saved.") {
		t.Fatalf("running VM with live audio and forwards: %q", got)
	}
	if got := settingsFooterText(true, false, true); !strings.HasSuffix(got, "Port forwards apply when saved.") {
		t.Fatalf("running VM with live forwards: %q", got)
	}
	if forwardsHelpText(true) == forwardsHelpText(false) || !strings.Contains(forwardsHelpText(false), "next launch") {
		t.Fatalf("forward help does not follow the running VM")
	}
}

// TestHostAppOffersLeaveOutAllowedAppsAndTellTwinsApart pins what the Add an
// app dropdown lists.
func TestHostAppOffersLeaveOutAllowedAppsAndTellTwinsApart(t *testing.T) {
	installed := []hostApp{{"calc.desktop", "Calculator"}, {"org.example.Files.desktop", "Files"}, {"nemo.desktop", "Files"}, {"zed.desktop", "Zed"}}
	labels, offers := hostAppOffersFor([]hostApp{{"zed.desktop", "Zed"}, {"gone.desktop", "Uninstalled"}}, installed)
	if want := []string{"Calculator", "Files (org.example.Files)", "Files (nemo)"}; !reflect.DeepEqual(labels, want) {
		t.Fatalf("labels %q, want %q", labels, want)
	}
	if !reflect.DeepEqual(offers, installed[:3]) {
		t.Fatalf("offers %+v", offers)
	}
	labels, offers = hostAppOffersFor(installed, installed)
	if len(offers) != 0 || !reflect.DeepEqual(labels, []string{noHostAppsLabel}) {
		t.Fatalf("nothing left: %q %+v", labels, offers)
	}
	if kept := withoutHostApp(installed, "nemo.desktop"); len(kept) != 3 || kept[2].ID != "zed.desktop" {
		t.Fatalf("kept %+v", kept)
	}
}

// TestFullscreenDisplayChoicesUseConnectorNames pins that the saved choice is
// the connector SDL matches, and that an unusable name is never offered.
func TestFullscreenDisplayChoicesUseConnectorNames(t *testing.T) {
	got := fullscreenDisplayChoices([]displayChoice{
		{Connector: "DP-2", Description: "Dell U2720Q"},
		{Connector: "eDP-1", Description: "eDP-1"},
		{Connector: "HDMI-A-1"},
		{Description: "Unnamed panel"},
		{Connector: "DP-1,DP-2", Description: "Bad name"},
	})
	want := []audioDevice{
		{Name: "DP-2", Label: "Dell U2720Q (DP-2)"},
		{Name: "eDP-1", Label: "eDP-1"},
		{Name: "HDMI-A-1", Label: "HDMI-A-1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("choices = %#v, want %#v", got, want)
	}
	labels, names, index := namedChoices("Automatic", "Not connected: ", got, "DP-3")
	if names[index] != "DP-3" || labels[index] != "Not connected: DP-3" {
		t.Fatalf("a disconnected choice is kept: %v %v %d", labels, names, index)
	}
}
