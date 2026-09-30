package main

import (
	"fmt"
	"strings"
)

// The pieces of the window that hold rules rather than widgets live here, so
// they can be tested without a display.

type row struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	State  string `json:"state,omitempty"`
}

type section struct {
	Heading string `json:"heading"`
	Rows    []row  `json:"rows"`
}

// A button the launcher asked for. Reply is what the window sends back.
type action struct {
	Label       string `json:"label"`
	Reply       string `json:"reply"`
	Suggested   bool   `json:"suggested"`
	Destructive bool   `json:"destructive"`
}

// escapeReply is what Escape answers for a prompt, or "" when it should do
// nothing. Progress screens ignore it: a stray key must never stop a download
// or shut Omarchy down. Prompts that mean "cancel setup" when closed are left
// to the close button so setup is never abandoned by accident.
func escapeReply(s state) string {
	if s.NonCancellable {
		return ""
	}
	switch s.Prompt {
	case "home":
		return "close"
	case "about", "settings-saved", "recovery":
		return "back"
	case "close", "forget-location", "delete-default":
		return "keep"
	case "error":
		return "close"
	case "settings", "grant-files", "backup-folder", "restore-archive", "restore-parent", "attach-folder", "move-folder", "choice":
		return "cancel"
	}
	return ""
}

// errorActions is the button row of a failure the launcher can retry.
func errorActions(s state) []action {
	var actions []action
	if s.CanRetry {
		actions = append(actions, action{Label: "Try again", Reply: "retry", Suggested: true})
	}
	return append(actions, action{Label: "Close", Reply: "close"})
}

func percentText(current, total int64) string {
	if total <= 0 {
		return ""
	}
	fraction := min(max(float64(current)/float64(total), 0), 1)
	return fmt.Sprintf("%.0f%%", fraction*100)
}

// versionLabel displays the backend version using the shared product typography.
func versionLabel(version string) string {
	version = strings.TrimPrefix(version, "v")
	return strings.NewReplacer("-preview.", " preview ", "-", " ").Replace(version)
}

// platformVersion joins the platform and backend version on one identity line.
func platformVersion(version string) string {
	if version == "" {
		return ""
	}
	return " · v" + versionLabel(version)
}

// homeSectionsForState keeps integration choices visible, like the Mac launcher.
// The longer first setup explanation is optional; existing VM storage stays visible.
func homeSectionsForState(s state) (visible, setup []section) {
	if s.Prompt != "home" {
		return nil, nil
	}
	for _, group := range s.Sections {
		if group.Heading == "What setup does" {
			setup = append(setup, group)
		} else {
			visible = append(visible, group)
		}
	}
	return
}

// homeSuggested is the action the window focuses first.
func homeSuggested(actions []action) int {
	for i, a := range actions {
		if a.Suggested {
			return i
		}
	}
	return 0
}

// sectionsText flattens sections into plain text for the terminal fallback and
// for tests that compare what was shown.
func sectionsText(sections []section) string {
	var b strings.Builder
	for _, s := range sections {
		if s.Heading != "" {
			b.WriteString(s.Heading + "\n")
		}
		for _, r := range s.Rows {
			b.WriteString(r.Title + ": " + r.Detail + "\n")
		}
	}
	return b.String()
}

// settingsFooterText keeps operation timing visible without repeating row help.
func settingsFooterText(running, audioLive, forwardsLive bool) string {
	text := "VM settings apply on the next launch."
	if running {
		text = "Shut down Omarchy and launch again to apply VM settings."
	}
	switch {
	case audioLive && forwardsLive:
		text += " Audio devices and port forwards apply when saved."
	case audioLive:
		text += " Audio device choices apply when saved."
	case forwardsLive:
		text += " Port forwards apply when saved."
	}
	return text
}

// forwardsHelpText says when edits to the forward list reach the VM.
func forwardsHelpText(live bool) string {
	if live {
		return "Each forward opens a port on 127.0.0.1. Changes apply when you save."
	}
	return "Each forward opens a port on 127.0.0.1. Changes apply on the next launch."
}
