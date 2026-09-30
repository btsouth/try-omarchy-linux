package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Run with TRYOMARCHY_UI_TEST=1 inside an isolated desktop. This inspects the
// real frontend, rather than a second approximation of its layout.
func TestNativeFooterAndReply(t *testing.T) {
	if os.Getenv("TRYOMARCHY_UI_TEST") != "1" {
		t.Skip("requires an isolated native GTK desktop")
	}
	r, w := io.Pipe()
	updates := make(chan state, 1)
	go func() {
		defer w.Close()
		for s := range updates {
			if json.NewEncoder(w).Encode(s) != nil {
				return
			}
		}
	}()
	var output bytes.Buffer
	cases := []struct {
		state  state
		button string
	}{
		{state{Prompt: "home", Actions: []action{{Label: "Launch Omarchy", Reply: "launch", Suggested: true}, {Label: "Settings", Reply: "settings"}, {Label: "Backup and recovery", Reply: "recovery"}, {Label: "Close", Reply: "close"}}, Sections: []section{{Heading: "Storage", Rows: []row{{Title: "Location", Detail: strings.Repeat("Long folder/", 30)}}}}, Installed: true}, "Launch Omarchy"},
		{state{Prompt: "settings", Settings: &settingsForm{ResourceProfile: "manual", CPUMax: 8, Memory: "6144", CPUs: "3"}}, "Save settings"},
		{state{Prompt: "settings", Notice: "Could not save resource profile.", Settings: &settingsForm{ResourceProfile: "manual", CPUMax: 8, Memory: "0", CPUs: "0"}}, "Save settings"},
		{state{Prompt: "account"}, "Set up my own account"},
		{state{Prompt: "error", CanRetry: true}, "Try again"},
		{state{Prompt: "recovery", CanMove: true, CanReset: true, CanCleanMove: true, CanCleanReset: true, CanCleanLeftovers: true}, "Back"},
		{state{Prompt: "about", Sections: []section{{Heading: "Settings and devices", Rows: []row{{Title: "When changes apply", Detail: strings.Repeat("Long help text ", 40)}}}}}, "Back"},
		{state{Prompt: "home", Version: "v0.1.0-preview.5", Headline: "Welcome to Try Omarchy", Sections: []section{{Heading: "What setup does", Rows: []row{{Title: "Account", Detail: "Choose your own account or quick start."}}}, {Heading: "Integrations", Rows: []row{{Title: "Resources", Detail: "Balanced · checked at the next launch", State: "pending"}}}}, Actions: []action{{Label: "Set up Omarchy", Reply: "setup", Suggested: true}, {Label: "Choose location...", Reply: "customize"}, {Label: "Settings", Reply: "settings"}, {Label: "Close", Reply: "close"}}}, "Set up Omarchy"},
		{state{Prompt: "location", Sections: []section{{Rows: []row{{Title: "Default location", Detail: strings.Repeat("Long folder/", 30)}}}}}, "Use default location"},
		{state{Prompt: "share"}, "Not now"},
		{state{Prompt: "choice", Title: "Reset Omarchy?", Primary: "Keep this VM", Secondary: "Reset", Destructive: true}, "Keep this VM"},
		{state{Prompt: "close"}, "Keep running"},
		{state{Prompt: "delete-default"}, "Keep this VM"},
		{state{Status: "Downloading Omarchy", Current: 42, Total: 100}, "Cancel"},
		{state{Status: "Starting Omarchy", Booting: true}, "Stop Omarchy"},
	}
	index, inspecting := 0, false
	runUI(r, &output, func(window *adw.ApplicationWindow) {
		t.Logf("native high contrast: %t", adw.StyleManagerGetDefault().HighContrast())
		icons := gtk.IconThemeGetForDisplay(window.Window.Widget.Display())
		for _, name := range []string{"object-select-symbolic", "action-unavailable-symbolic", "content-loading-symbolic", "dialog-error-symbolic"} {
			if !icons.HasIcon(name) {
				t.Errorf("integration or failure icon is unavailable: %s", name)
			}
		}
		glib.TimeoutAdd(250, func() bool {
			if index == len(cases) {
				close(updates)
				return false
			}
			if !inspecting {
				s := cases[index].state
				s.Request = uint64(index + 1)
				updates <- s
				inspecting = true
				return true
			}
			var found *gtk.Button
			var memory *gtk.SpinButton
			var autoMemory *gtk.CheckButton
			var refresh *gtk.Button
			brandVisible, integrationsVisible := false, false
			noticeVisible := false
			var walk func(*gtk.Widget)
			walk = func(widget *gtk.Widget) {
				if !widget.Visible() && widget.CSSName() != "spinbutton" {
					return
				}
				if widget.CSSName() == "flowboxchild" && widget.Focusable() {
					t.Error("secondary action wrapper adds an inactive keyboard focus stop")
				}
				if l, ok := widget.Object.Cast().(*gtk.Label); ok {
					if l.Text() == "OMARCHY · LINUX" {
						brandVisible = true
					}
					if l.Text() == "Resources" && cases[index].state.Prompt == "home" {
						integrationsVisible = true
						for parent := widget.Parent(); parent != nil; parent = gtk.BaseWidget(parent).Parent() {
							if gtk.BaseWidget(parent).CSSName() == "expander" {
								integrationsVisible = false
								break
							}
						}
					}
				}
				if notice, ok := widget.Object.Cast().(*gtk.Label); ok && cases[index].state.Notice != "" && notice.Text() == cases[index].state.Notice {
					bounds, ok := notice.ComputeBounds(&window.Window)
					noticeVisible = ok && bounds.X() >= 0 && bounds.Y() >= 0 && bounds.X()+bounds.Width() <= float32(window.Width())+1 && bounds.Y()+bounds.Height() <= float32(window.Height())+1
				}
				if scroller, ok := widget.Object.Cast().(*gtk.ScrolledWindow); ok {
					adjustment := scroller.HAdjustment()
					if adjustment.Upper()-adjustment.PageSize() > 1 {
						t.Errorf("%s: content requires horizontal scrolling (%.0f > %.0f)", cases[index].state.Prompt, adjustment.Upper(), adjustment.PageSize())
					}
					if index != 2 && scroller.VAdjustment().Value() > 1 {
						t.Errorf("%s: a new page inherited the preceding page's scroll position", cases[index].state.Prompt)
					}
					if index == 6 {
						// Read About to the end before opening another surface.
						scroller.VAdjustment().SetValue(scroller.VAdjustment().Upper())
					}
				}
				if spin, ok := widget.Object.Cast().(*gtk.SpinButton); ok && spin.Digits() == 2 {
					memory = spin
				}
				if check, ok := widget.Object.Cast().(*gtk.CheckButton); ok && check.Label() == "Choose memory automatically" {
					autoMemory = check
				}
				if b, ok := widget.Object.Cast().(*gtk.Button); ok {
					if b.Label() == "Refresh devices" {
						refresh = b
					}
					if b.Label() == cases[index].button {
						found = b
					}
				}
				for child := widget.FirstChild(); child != nil; child = gtk.BaseWidget(child).NextSibling() {
					walk(gtk.BaseWidget(child))
				}
			}
			walk(&window.Window.Widget)
			if !brandVisible {
				t.Errorf("%s: shared product header is missing", cases[index].state.Prompt)
			}
			if cases[index].state.Headline == "Welcome to Try Omarchy" && !integrationsVisible {
				t.Error("first-use integration choices are hidden inside the setup expander")
			}
			if cases[index].state.Notice != "" && !noticeVisible {
				t.Error("Settings failure notice is not visible inside the window")
			}
			if found == nil {
				t.Errorf("%s: no %q button", cases[index].state.Prompt, cases[index].button)
			} else if bounds, ok := found.ComputeBounds(&window.Window); !ok || bounds.Y() < 0 || bounds.Y()+bounds.Height() > float32(window.Height())+1 || bounds.X() < 0 || bounds.X()+bounds.Width() > float32(window.Width())+1 {
				t.Errorf("%s: footer button is outside %dx%d window", cases[index].state.Prompt, window.Width(), window.Height())
			} else {
				t.Logf("%s: %q visible inside %dx%d", cases[index].state.Prompt, cases[index].button, window.Width(), window.Height())
			}
			if window.HasCSSClass("try-omarchy") == adw.StyleManagerGetDefault().HighContrast() {
				t.Error("brand override did not follow high contrast")
			}
			if index == 1 && memory != nil && autoMemory != nil {
				memory.SetValue(8)
				autoMemory.SetActive(true)
				if memory.Visible() {
					t.Error("automatic memory did not hide numeric tuning")
				}
			}
			if index == 1 && refresh != nil {
				refresh.GrabFocus()
				refresh.Emit("clicked")
			}
			if index == 2 && refresh != nil {
				focus := window.Window.Focus()
				if focus == nil || gtk.BaseWidget(focus).Object.Native() != refresh.Object.Native() {
					t.Error("device refresh did not restore the focused Refresh button")
				}
			}
			if index == 2 && memory != nil && autoMemory != nil {
				autoMemory.SetActive(false)
				if memory.Value() != 8 || !memory.Visible() {
					t.Error("device refresh discarded hidden manual memory")
				}
			}
			if index == 0 && found != nil {
				// Duplicate activation must produce one reply for this request.
				found.Emit("clicked")
				found.Emit("clicked")
			}
			index++
			inspecting = false
			return true
		})
	})
	if got := strings.Count(output.String(), `"event":"reply"`); got != 2 {
		t.Fatalf("home activation and device refresh emitted %d replies: %s", got, output.String())
	}
}

// A compositor sends Return after the ready marker appears. This checks the
// native multiline control against the real window default action.
func TestNativeMultilineEnter(t *testing.T) {
	marker := os.Getenv("TRYOMARCHY_UI_KEYBOARD_READY")
	if os.Getenv("TRYOMARCHY_UI_TEST") != "1" || marker == "" {
		t.Skip("requires external keys in an isolated desktop")
	}
	r, w := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer w.Close()
		json.NewEncoder(w).Encode(state{Request: 1, Prompt: "settings", Settings: &settingsForm{ResourceProfile: "balanced", CPUMax: 4}})
		<-done
	}()
	var output bytes.Buffer
	ready, ticks := false, 0
	runUI(r, &output, func(window *adw.ApplicationWindow) {
		glib.TimeoutAdd(100, func() bool {
			ticks++
			var text *gtk.TextView
			var walk func(*gtk.Widget)
			walk = func(widget *gtk.Widget) {
				if expander, ok := widget.Object.Cast().(*gtk.Expander); ok && expander.Label() == "Advanced network settings" {
					expander.SetExpanded(true)
				}
				if view, ok := widget.Object.Cast().(*gtk.TextView); ok {
					text = view
				}
				for child := widget.FirstChild(); child != nil; child = gtk.BaseWidget(child).NextSibling() {
					walk(gtk.BaseWidget(child))
				}
			}
			walk(&window.Window.Widget)
			if text != nil && text.Mapped() && !ready {
				if !text.GrabFocus() {
					return true
				}
				if err := os.WriteFile(marker, []byte("ready\n"), 0600); err != nil {
					t.Error(err)
				}
				ready = true
			}
			entered := false
			if text != nil {
				start, end := text.Buffer().Bounds()
				entered = text.Buffer().Text(start, end, true) == "\n"
			}
			if entered || ticks == 200 {
				if !entered {
					t.Error("Return did not insert a newline in the focused multiline field")
				}
				if strings.Contains(output.String(), `"event":"reply"`) {
					t.Error("Return in a multiline field submitted Settings")
				}
				close(done)
				return false
			}
			return true
		})
	})
}
