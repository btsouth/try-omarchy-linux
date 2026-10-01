package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
		{state{Prompt: "account"}, "Continue"},
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
		{state{Status: "Waiting for the Omarchy desktop", Detail: "Omarchy is running. Finish account setup or sign in in the Omarchy window. If the guest is stuck, use Stop Omarchy and try again; diagnostics are in the data folder. The launcher closes when the desktop is ready.", Booting: true}, "Stop Omarchy"},
		{state{Prompt: "home", Installed: true, CheckAgain: true, Headline: "Saved settings need attention.", Notice: "Saved settings could not be read.", HelpURL: "https://github.com/btsouth/try-omarchy-linux/blob/master/docs/LINUX-HELP.md#settings", Actions: []action{{Label: "Check again", Reply: "check", Suggested: true}, {Label: "Settings", Reply: "settings"}, {Label: "Backup and recovery", Reply: "recovery"}, {Label: "Close", Reply: "close"}}, Sections: []section{{Heading: "Integrations", Rows: []row{{Title: "Settings", Detail: "Saved choices could not be read. Open Settings to see the problem.", State: "unavailable"}}}}}, "Check again"},
		{state{Prompt: "settings", Notice: "Could not save startup. Already saved: VM configuration and audio devices. Your remaining edits are kept here. Check that the VM folder is writable and has free space, then Save again.", Settings: &settingsForm{Running: true, AudioLive: true, ForwardsLive: true, ResourceProfile: "balanced", CPUMax: 8}}, "Save settings"},
		{state{Prompt: "settings", Settings: &settingsForm{Running: true, ResourceProfile: "balanced", CPUMax: 8, Reclaim: &reclaimInfo{Status: "Omarchy's disk uses 9.4 GB on this drive. Reclaim gives back space from files you deleted inside Omarchy.", CanStart: true}}}, "Save settings"},
		{state{Prompt: "settings", Settings: &settingsForm{Running: true, ResourceProfile: "balanced", CPUMax: 8, Reclaim: &reclaimInfo{Status: "Preparing free space. Keep Omarchy running until preparation finishes."}}}, "Save settings"},
		{state{Prompt: "message", Title: "Reclaim disk space", Status: "Preparing free space. Keep Omarchy running until it finishes, then shut it down to give the space back."}, "Done"},
	}
	index, inspecting := 0, false
	pendingCapture := ""
	runUI(r, &output, func(window *adw.ApplicationWindow) {
		t.Logf("native high contrast: %t", adw.StyleManagerGetDefault().HighContrast())
		icons := gtk.IconThemeGetForDisplay(window.Window.Widget.Display())
		for _, name := range []string{"dialog-error-symbolic"} {
			if !icons.HasIcon(name) {
				t.Errorf("required failure icon is unavailable: %s", name)
			}
		}
		glib.TimeoutAdd(250, func() bool {
			if pendingCapture != "" {
				if err := captureNative(window, pendingCapture); err != nil {
					t.Errorf("capture %s: %v", pendingCapture, err)
				}
				pendingCapture = ""
			}
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
			var refresh, reclaim *gtk.Button
			var pages *gtk.Stack
			brandVisible, integrationsVisible := false, false
			noticeVisible := false
			var walk func(*gtk.Widget)
			walk = func(widget *gtk.Widget) {
				if stack, ok := widget.Object.Cast().(*gtk.Stack); ok && stack.HasCSSClass("settings-pages") {
					pages = stack
				}
				if widget.CSSName() == "flowboxchild" && widget.Focusable() {
					t.Error("secondary action wrapper adds an inactive keyboard focus stop")
				}
				if l, ok := widget.Object.Cast().(*gtk.Label); ok {
					if l.Mapped() && l.Text() != "" {
						for parent := widget.Parent(); parent != nil; parent = gtk.BaseWidget(parent).Parent() {
							if b, ok := gtk.BaseWidget(parent).Object.Cast().(*gtk.Button); ok {
								if l.Layout().LineCount() > len(strings.Fields(l.Text())) {
									t.Errorf("%s: action %q breaks words into character lines", cases[index].state.Prompt, b.Label())
								}
								break
							}
						}
					}
					if strings.HasPrefix(l.Text(), "LINUX") && l.Mapped() {
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
				if scroller, ok := widget.Object.Cast().(*gtk.ScrolledWindow); ok && scroller.Mapped() {
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
					if b.Label() == "Prepare free space" {
						reclaim = b
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
			if settings := cases[index].state.Settings; cases[index].state.Prompt == "settings" && reclaim != nil {
				if wantShown := settings.Reclaim != nil; reclaim.Mapped() != wantShown {
					t.Errorf("settings %d: reclaim card shown=%v, want %v", index, reclaim.Mapped(), wantShown)
				} else if wantShown && reclaim.Sensitive() != settings.Reclaim.CanStart {
					t.Errorf("settings %d: reclaim button sensitive=%v, want %v", index, reclaim.Sensitive(), settings.Reclaim.CanStart)
				}
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
				pages.SetVisibleChildName("Devices")
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
			// Capture on the next tick, after layout settles. Bring the reclaim
			// card into view on the pages that show it.
			pendingCapture = fmt.Sprintf("footer-%02d-%s", index, orDefault(cases[index].state.Prompt, "progress"))
			if settings := cases[index].state.Settings; settings != nil && settings.Reclaim != nil && reclaim != nil {
				reclaim.GrabFocus()
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

// A failed save must reveal its full reason and recovery guidance on the page
// that submitted it, while retaining edits and the fixed Save action.
func TestNativeSettingsFailureAcrossPages(t *testing.T) {
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
	form := &settingsForm{ResourceProfile: "manual", CPUMax: 8, Memory: "8192", CPUs: "5", SSHPort: "2222", Forwards: "tcp:8080:80"}
	partial := "Could not save startup. Already saved: VM configuration and audio devices. Your remaining edits are kept here. Check that the VM folder is writable and has free space, then Save again."
	validation := "SSH host port must be between 1024 and 65535."
	updates <- state{Request: 901, Prompt: "settings", Settings: form}
	var output bytes.Buffer
	phase, ticks := 0, 0
	runUI(r, &output, func(window *adw.ApplicationWindow) {
		glib.TimeoutAdd(250, func() bool {
			ticks++
			if ticks > 60 {
				t.Error("Settings failure inspection timed out")
				close(updates)
				return false
			}
			var stack *gtk.Stack
			var scroller *gtk.ScrolledWindow
			var save *gtk.Button
			var problem *gtk.Label
			var walk func(*gtk.Widget)
			walk = func(widget *gtk.Widget) {
				switch control := widget.Object.Cast().(type) {
				case *gtk.Stack:
					if control.HasCSSClass("settings-pages") {
						stack = control
					}
				case *gtk.ScrolledWindow:
					if control.Mapped() {
						scroller = control
					}
				case *gtk.Button:
					if control.Label() == "Save settings" && control.Mapped() {
						save = control
					}
				case *gtk.Label:
					if control.HasCSSClass("settings-problem") {
						problem = control
					}
				}
				for child := widget.FirstChild(); child != nil; child = gtk.BaseWidget(child).NextSibling() {
					walk(gtk.BaseWidget(child))
				}
			}
			walk(&window.Window.Widget)
			if stack == nil || scroller == nil || save == nil || problem == nil {
				return true
			}
			checkFailure := func(page, message string) {
				if stack.VisibleChildName() != page || !problem.Mapped() || !problem.Selectable() || problem.Text() != message {
					t.Errorf("%s: detailed save failure is unavailable on the submitting page", page)
				}
				bounds, ok := problem.ComputeBounds(scroller)
				if !ok || bounds.Y() < 0 || bounds.Y() >= float32(scroller.Height()) || bounds.X() < 0 || bounds.X()+bounds.Width() > float32(scroller.Width())+1 || scroller.VAdjustment().Value() != 0 {
					t.Errorf("%s: save failure was left below the visible scroll viewport", page)
				}
				if !save.Sensitive() {
					t.Error("failed save cannot be retried")
				}
			}
			switch phase {
			case 0, 3:
				scroller.VAdjustment().SetValue(scroller.VAdjustment().Upper())
			case 1:
				save.Emit("clicked")
				updates <- state{Request: 902, Prompt: "settings", Notice: "Could not save startup.", Status: partial, Settings: form}
			case 2:
				checkFailure("General", partial)
				stack.SetVisibleChildName("Advanced")
			case 4:
				save.Emit("clicked")
				updates <- state{Request: 903, Prompt: "settings", Notice: "Check your settings before saving.", Status: validation, Settings: form}
			case 5:
				checkFailure("Advanced", validation)
				close(updates)
				return false
			}
			phase++
			return true
		})
	})
	var replies int
	for _, line := range bytes.Split(output.Bytes(), []byte("\n")) {
		var event struct {
			Event string `json:"event"`
			Value string `json:"value"`
		}
		if json.Unmarshal(line, &event) == nil && event.Event == "reply" {
			replies++
			var saved settingsForm
			if err := json.Unmarshal([]byte(event.Value), &saved); err != nil {
				t.Fatal(err)
			}
			if saved.Memory != "8192" || saved.CPUs != "5" || saved.SSHPort != "2222" || saved.Forwards != "tcp:8080:80" {
				t.Errorf("failed save lost edits: %+v", saved)
			}
		}
	}
	if replies != 2 {
		t.Errorf("expected two save attempts, got %d", replies)
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
				if stack, ok := widget.Object.Cast().(*gtk.Stack); ok && stack.HasCSSClass("settings-pages") {
					stack.SetVisibleChildName("Advanced")
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

// Switching settings pages must preserve edits in hidden controls and keep the
// product identity and actions outside the scrolling content.
func TestNativeSettingsPagesAndAccountChoices(t *testing.T) {
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
	updates <- state{Request: 801, Prompt: "settings", Version: "v0.1.0-preview.5", Settings: &settingsForm{
		ResourceProfile: "manual", CPUMax: 8, Memory: "6144", CPUs: "3", Microphone: true,
		Camera: true, CameraID: "missing-camera", AudioOutput: "missing-output", AudioInput: "missing-input",
		Scale: "1.5", Keyboard: "de", SSHEnabled: true, SSHPort: "2222", SSHKey: "/tmp/test-key.pub",
		Forwards: "tcp:8080:80", DiskGiB: "48", Share: "/run/user/1000/doc/test-grant/test-share", ShareDisplay: "~/test-share", ShareEnabled: true,
	}}
	phase, ticks := 0, 0
	var headerY float32
	runUI(r, &output, func(window *adw.ApplicationWindow) {
		glib.TimeoutAdd(250, func() bool {
			ticks++
			if ticks > 60 {
				t.Error("native page/account inspection timed out")
				close(updates)
				return false
			}
			var stack *gtk.Stack
			var header *gtk.Box
			buttons := map[string]*gtk.Button{}
			checks := map[string]*gtk.CheckButton{}
			var memory *gtk.SpinButton
			var cpus *gtk.SpinButton
			var shareDisplay *gtk.Entry
			var settingScroll *gtk.ScrolledWindow
			var walk func(*gtk.Widget)
			walk = func(widget *gtk.Widget) {
				if entry, ok := widget.Object.Cast().(*gtk.Entry); ok && entry.Mapped() && !entry.Editable() {
					shareDisplay = entry
				}
				if box, ok := widget.Object.Cast().(*gtk.Box); ok && box.HasCSSClass("product-header") {
					header = box
				}
				if pages, ok := widget.Object.Cast().(*gtk.Stack); ok && pages.HasCSSClass("settings-pages") {
					stack = pages
				}
				if button, ok := widget.Object.Cast().(*gtk.Button); ok && button.Mapped() {
					buttons[button.Label()] = button
				}
				if check, ok := widget.Object.Cast().(*gtk.CheckButton); ok {
					checks[check.Label()] = check
				}
				if spin, ok := widget.Object.Cast().(*gtk.SpinButton); ok {
					if spin.Digits() == 2 {
						memory = spin
					} else if spin.Adjustment().Upper() == 8 {
						cpus = spin
					}
				}
				if scroller, ok := widget.Object.Cast().(*gtk.ScrolledWindow); ok && scroller.Mapped() {
					settingScroll = scroller
					if scroller.HAdjustment().Upper()-scroller.HAdjustment().PageSize() > 1 {
						t.Error("page content requires horizontal scrolling")
					}
				}
				for child := widget.FirstChild(); child != nil; child = gtk.BaseWidget(child).NextSibling() {
					walk(gtk.BaseWidget(child))
				}
			}
			walk(&window.Window.Widget)
			if header == nil || !header.Mapped() || stack == nil {
				return true
			}
			bounds, ok := header.ComputeBounds(&window.Window)
			if !ok || bounds.Y() < 0 || bounds.Y()+bounds.Height() > float32(window.Height()) {
				t.Error("shared header is outside the window")
			}
			if phase > 0 && bounds.Y() != headerY {
				t.Error("scrolling/settings navigation moved the shared product header")
			}
			assertFooter := func(label string) {
				button := buttons[label]
				if button == nil {
					t.Errorf("%q footer action is not mapped", label)
					return
				}
				b, ok := button.ComputeBounds(&window.Window)
				if !ok || b.Y()+b.Height() > float32(window.Height())+1 || b.X()+b.Width() > float32(window.Width())+1 {
					t.Errorf("%q footer action is outside the window", label)
				}
			}
			switch phase {
			case 0:
				if buttons["Save settings"] == nil {
					return true
				}
				if shareDisplay == nil || shareDisplay.Text() != "~/test-share" {
					t.Error("Shared folder did not show its readable name")
				}
				headerY = bounds.Y()
				if stack.VisibleChildName() != "General" || checks["Allow camera access"].Mapped() {
					t.Error("Settings did not begin on its General page")
				}
				memory.SetValue(8)
				cpus.SetValue(5)
				checks["Open fullscreen"].SetActive(true)
				settingScroll.VAdjustment().SetValue(settingScroll.VAdjustment().Upper())
				assertFooter("Save settings")
			case 1:
				assertFooter("Save settings")
				stack.SetVisibleChildName("Devices")
			case 2:
				if !checks["Allow camera access"].Mapped() || memory.Mapped() {
					t.Error("Devices page did not isolate its own controls")
				}
				if settingScroll.VAdjustment().Value() != 0 {
					t.Error("Devices inherited General's scroll position")
				}
				checks["Allow microphone access"].SetActive(false)
				assertFooter("Save settings")
				stack.SetVisibleChildName("Advanced")
			case 3:
				if !checks["Allow SSH from this computer"].Mapped() || checks["Allow camera access"].Mapped() {
					t.Error("Advanced page did not isolate its own controls")
				}
				assertFooter("Save settings")
				stack.SetVisibleChildName("General")
			case 4:
				if memory.Value() != 8 || cpus.ValueAsInt() != 5 || !checks["Open fullscreen"].Active() {
					t.Error("switching settings pages lost resource/display edits")
				}
				buttons["Save settings"].Emit("clicked")
				buttons["Save settings"].Emit("clicked")
				updates <- state{Request: 802, Prompt: "account", Version: "v0.1.0-preview.5"}
			case 5:
				if buttons["Continue"] == nil {
					return true
				}
				if !checks["My own username and password (recommended)"].Active() || checks["Quick start (omarchy / omarchy)"].Active() {
					t.Error("personal account is not the first-use default")
				}
				assertFooter("Continue")
				buttons["Continue"].Emit("clicked")
				updates <- state{Request: 803, Prompt: "home", Actions: []action{{Label: "Close", Reply: "close"}}}
			case 6:
				updates <- state{Request: 804, Prompt: "account"}
			case 7:
				if buttons["Continue"] == nil {
					return true
				}
				quick := checks["Quick start (omarchy / omarchy)"]
				quick.SetActive(true)
				if checks["My own username and password (recommended)"].Active() {
					t.Error("account choices are not mutually exclusive")
				}
				buttons["Continue"].Emit("clicked")
				buttons["Continue"].Emit("clicked")
				close(updates)
				return false
			}
			phase++
			return true
		})
	})
	var replies []struct {
		Event   string `json:"event"`
		Request uint64 `json:"request"`
		Value   string `json:"value"`
	}
	for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n")) {
		var event struct {
			Event   string `json:"event"`
			Request uint64 `json:"request"`
			Value   string `json:"value"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		if event.Event == "reply" {
			replies = append(replies, event)
		}
	}
	if len(replies) != 3 || replies[0].Request != 801 || replies[1].Value != "personal" || replies[2].Value != "instant" {
		t.Fatalf("unexpected settings/account replies: %+v", replies)
	}
	var saved settingsForm
	if err := json.Unmarshal([]byte(replies[0].Value), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Memory != "8192" || saved.CPUs != "5" || !saved.Fullscreen || saved.Microphone || saved.CameraID != "missing-camera" || saved.AudioOutput != "missing-output" || saved.AudioInput != "missing-input" || saved.Scale != "1.5" || saved.Keyboard != "de" || saved.DiskGiB != "48" || saved.Share != "/run/user/1000/doc/test-grant/test-share" || !saved.ShareEnabled || saved.SSHKey != "/tmp/test-key.pub" || saved.SSHPort != "2222" || saved.Forwards != "tcp:8080:80" {
		t.Fatalf("hidden settings/edits were not preserved: %+v", saved)
	}
}

// A real compositor sends Return to the selected radio control. The native
// radio must not consume it instead of activating the request's Continue action.
func TestNativeAccountEnter(t *testing.T) {
	marker := os.Getenv("TRYOMARCHY_UI_KEYBOARD_READY")
	if os.Getenv("TRYOMARCHY_UI_TEST") != "1" || marker == "" {
		t.Skip("requires external keys in an isolated desktop")
	}
	r, w := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer w.Close()
		json.NewEncoder(w).Encode(state{Request: 701, Prompt: "account"})
		<-done
	}()
	var output bytes.Buffer
	ready, ticks := false, 0
	quick := os.Getenv("TRYOMARCHY_UI_ACCOUNT_QUICK") == "1"
	expected := "personal"
	choice := "My own username and password (recommended)"
	if quick {
		expected, choice = "instant", "Quick start (omarchy / omarchy)"
	}
	runUI(r, &output, func(window *adw.ApplicationWindow) {
		glib.TimeoutAdd(100, func() bool {
			ticks++
			var walk func(*gtk.Widget)
			walk = func(widget *gtk.Widget) {
				if radio, ok := widget.Object.Cast().(*gtk.CheckButton); ok && radio.Label() == choice && radio.Mapped() && !ready {
					radio.SetActive(true)
					if radio.GrabFocus() {
						if err := os.WriteFile(marker, []byte("ready\n"), 0600); err != nil {
							t.Error(err)
						}
						ready = true
					}
				}
				for child := widget.FirstChild(); child != nil; child = gtk.BaseWidget(child).NextSibling() {
					walk(gtk.BaseWidget(child))
				}
			}
			walk(&window.Window.Widget)
			replied := strings.Contains(output.String(), `"event":"reply"`)
			if replied || ticks == 200 {
				if !strings.Contains(output.String(), `"value":"`+expected+`"`) || strings.Count(output.String(), `"event":"reply"`) != 1 {
					t.Errorf("Return did not continue exactly once with %s: %s", expected, output.String())
				}
				close(done)
				return false
			}
			return true
		})
	})
}
