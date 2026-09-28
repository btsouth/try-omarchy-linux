package main

import (
	"reflect"
	"strings"
	"testing"
)

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
}

func TestSectionsFlattenForTheTerminalFallback(t *testing.T) {
	text := sectionsText([]section{{Heading: "Storage", Rows: []row{{Title: "Location", Detail: "~/Omarchy"}}}, {Rows: []row{{Title: "Free", Detail: "9 GB"}}}})
	for _, want := range []string{"Storage\n", "Location: ~/Omarchy\n", "Free: 9 GB\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}
