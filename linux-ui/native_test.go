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
		{state{Prompt: "settings", Settings: &settingsForm{ResourceProfile: "manual", CPUMax: 8, Memory: "0", CPUs: "0"}}, "Save settings"},
		{state{Prompt: "account"}, "Set up my own account"},
		{state{Prompt: "error", CanRetry: true}, "Try again"},
		{state{Prompt: "recovery", CanMove: true, CanReset: true, CanCleanMove: true, CanCleanReset: true, CanCleanLeftovers: true}, "Back"},
	}
	index, inspecting := 0, false
	runUI(r, &output, func(window *adw.ApplicationWindow) {
		t.Logf("native high contrast: %t", adw.StyleManagerGetDefault().HighContrast())
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
			var walk func(*gtk.Widget)
			walk = func(widget *gtk.Widget) {
				if !widget.Visible() && widget.CSSName() != "spinbutton" {
					return
				}
				if widget.CSSName() == "flowboxchild" && widget.Focusable() {
					t.Error("secondary action wrapper adds an inactive keyboard focus stop")
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
