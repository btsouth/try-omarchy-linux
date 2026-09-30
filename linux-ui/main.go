// try-omarchy-setup owns only the setup window. The launcher owns downloads,
// disks and the VM, and sends complete UI snapshots as newline-delimited JSON.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

type state struct {
	Status         string        `json:"status"`
	Current        int64         `json:"current"`
	Total          int64         `json:"total"`
	Error          bool          `json:"error"`
	ErrorTitle     string        `json:"errorTitle"`
	Booting        bool          `json:"booting"`
	NonCancellable bool          `json:"nonCancellable"`
	Prompt         string        `json:"prompt"`
	Request        uint64        `json:"request"`
	Path           string        `json:"path"`
	Version        string        `json:"version"`
	CanAttach      bool          `json:"canAttach"`
	CanForget      bool          `json:"canForget"`
	CanDelete      bool          `json:"canDelete"`
	Settings       *settingsForm `json:"settings"`
	// A "choice" prompt names its own title and two actions.
	Title             string `json:"title"`
	Primary           string `json:"primary"`
	Secondary         string `json:"secondary"`
	Destructive       bool   `json:"destructive"`
	CanMove           bool   `json:"canMove"`
	CanReset          bool   `json:"canReset"`
	CanCleanMove      bool   `json:"canCleanMove"`
	CanCleanReset     bool   `json:"canCleanReset"`
	CanCleanLeftovers bool   `json:"canCleanLeftovers"`
	// Plain-language content the launcher composes and this window only lays out.
	Headline   string    `json:"headline"`
	Notice     string    `json:"notice"`
	HelpURL    string    `json:"helpUrl"`
	Sections   []section `json:"sections"`
	Detail     string    `json:"detail"`
	CanRetry   bool      `json:"canRetry"`
	CheckAgain bool      `json:"checkAgain"`
	Installed  bool      `json:"installed"`
	Setup      string    `json:"setup"`
	Actions    []action  `json:"actions"`
	Menu       []action  `json:"menu"`
}

// Recovery keeps this window open after a cancelled copy. Ignore copy progress
// until the launcher returns to its home prompt, then make the window usable.
func acceptStateAfterCancel(cancelling bool, next state) bool {
	return !cancelling || next.Prompt == "home" || next.NonCancellable
}

type settingsForm struct {
	Memory             string        `json:"memory"`
	CPUs               string        `json:"cpus"`
	Render             string        `json:"render"`
	Fullscreen         bool          `json:"fullscreen"`
	Microphone         bool          `json:"microphone"`
	Camera             bool          `json:"camera"`
	CameraID           string        `json:"cameraID"`
	Cameras            []audioDevice `json:"cameras"`
	AudioOutput        string        `json:"audioOutput"`
	AudioInput         string        `json:"audioInput"`
	AudioOutputs       []audioDevice `json:"audioOutputs"`
	AudioInputs        []audioDevice `json:"audioInputs"`
	AudioLive          bool          `json:"audioLive"`
	RefreshAudio       bool          `json:"refreshAudio,omitempty"`
	DiskGiB            string        `json:"diskGiB"`
	Scale              string        `json:"scale"`
	Keyboard           string        `json:"keyboard"`
	SSHEnabled         bool          `json:"sshEnabled"`
	SSHPort            string        `json:"sshPort"`
	SSHKey             string        `json:"sshKey"`
	Forwards           string        `json:"forwards"`
	StartAutomatically bool          `json:"startAutomatically"`
	Share              string        `json:"share"`
	ShareEnabled       bool          `json:"shareEnabled"`
	CPUMax             int           `json:"cpuMax"`
	ClipboardShare     bool          `json:"clipboardShare"`
	ClipboardAvailable bool          `json:"clipboardAvailable"`
}

type audioDevice struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

func namedChoices(defaultLabel, unavailableLabel string, devices []audioDevice, selected string) (labels, names []string, index uint) {
	labels, names = []string{defaultLabel}, []string{""}
	for _, device := range devices {
		labels = append(labels, device.Label)
		names = append(names, device.Name)
		if device.Name == selected {
			index = uint(len(names) - 1)
		}
	}
	if selected != "" && index == 0 {
		labels = append(labels, unavailableLabel+selected)
		names = append(names, selected)
		index = uint(len(names) - 1)
	}
	return
}

// named gives a control the name a screen reader announces. The visible label
// above each settings field is a separate widget, so without this a keyboard
// or screen reader user meets nameless spin buttons and drop-downs.
func named(w gtk.Widgetter, label string) {
	w.(interface {
		UpdateProperty([]gtk.AccessibleProperty, []glib.Value)
	}).UpdateProperty([]gtk.AccessibleProperty{gtk.AccessiblePropertyLabel}, []glib.Value{*glib.NewValue(label)})
}

func clearChildren(box *gtk.Box) {
	for child := box.FirstChild(); child != nil; child = box.FirstChild() {
		box.Remove(child)
	}
}

// fillSections lays headed rows out as boxed lists. Rows show their value in
// the prominent line, the way a properties page does, and their text can be
// selected so a path can be copied.
func fillSections(box *gtk.Box, sections []section) {
	clearChildren(box)
	for _, s := range sections {
		group := adw.NewPreferencesGroup()
		if s.Heading != "" {
			group.SetTitle(s.Heading)
		}
		for _, r := range s.Rows {
			item := adw.NewActionRow()
			item.SetUseMarkup(false)
			item.SetTitle(r.Title)
			item.SetSubtitle(r.Detail)
			item.SetTitleLines(0)
			item.SetSubtitleLines(0)
			item.SetSubtitleSelectable(true)
			item.AddCSSClass("property")
			group.Add(item)
		}
		box.Append(group)
	}
	box.SetVisible(len(sections) > 0)
}

func main() {
	runtime.LockOSThread()
	glib.SetPrgname("com.tryomarchy.TryOmarchy")
	glib.SetApplicationName("Try Omarchy")
	app := adw.NewApplication("com.tryomarchy.TryOmarchy", gio.ApplicationNonUnique)
	app.ConnectActivate(func() {
		window := adw.NewApplicationWindow(&app.Application)
		window.SetTitle("Try Omarchy")
		window.SetDefaultSize(540, 620)
		window.SetResizable(true)
		layout := gtk.NewBox(gtk.OrientationVertical, 0)
		header := adw.NewHeaderBar()
		header.AddCSSClass("flat")
		// Less common home actions sit behind the header's overflow button.
		menuButton := gtk.NewMenuButton()
		menuButton.SetIconName("view-more-symbolic")
		menuButton.SetTooltipText("More actions")
		named(menuButton, "More actions")
		menuButton.SetVisible(false)
		menuPopover := gtk.NewPopover()
		menuBox := gtk.NewBox(gtk.OrientationVertical, 2)
		menuBox.SetMarginTop(6)
		menuBox.SetMarginBottom(6)
		menuBox.SetMarginStart(6)
		menuBox.SetMarginEnd(6)
		menuPopover.SetChild(menuBox)
		menuButton.SetPopover(menuPopover)
		header.PackEnd(menuButton)
		layout.Append(header)
		// A problem the user can fix, with a link to how.
		banner := adw.NewBanner("")
		banner.SetUseMarkup(false)
		banner.SetRevealed(false)
		layout.Append(banner)
		var bannerURL string
		openHelp := func(url string) {
			if url == "" {
				return
			}
			launcher := gtk.NewURILauncher(url)
			launcher.Launch(context.Background(), &window.Window, func(result gio.AsyncResulter) {
				if err := launcher.LaunchFinish(result); err != nil {
					fmt.Fprintln(os.Stderr, "Opening help:", err)
				}
			})
		}
		banner.ConnectButtonClicked(func() { openHelp(bannerURL) })
		page := adw.NewStatusPage()
		page.SetTitle("Try Omarchy")
		page.SetIconName("com.tryomarchy.TryOmarchy")
		page.SetVExpand(true)
		content := gtk.NewBox(gtk.OrientationVertical, 20)
		content.SetMarginStart(24)
		content.SetMarginEnd(24)
		content.SetMarginBottom(16)
		label := gtk.NewLabel("Preparing Omarchy...")
		label.SetWrap(true)
		label.SetWrapMode(pango.WrapWordChar)
		label.SetMaxWidthChars(48)
		label.SetJustify(gtk.JustifyCenter)
		content.Append(label)
		pageSections := gtk.NewBox(gtk.OrientationVertical, 12)
		pageSections.SetVisible(false)
		content.Append(pageSections)
		progress := gtk.NewProgressBar()
		named(progress, "Progress")
		content.Append(progress)
		detail := gtk.NewLabel("")
		detail.SetWrap(true)
		detail.SetWrapMode(pango.WrapWordChar)
		detail.SetMaxWidthChars(48)
		detail.SetJustify(gtk.JustifyCenter)
		detail.AddCSSClass("dim-label")
		detail.SetVisible(false)
		content.Append(detail)
		helpLink := gtk.NewLinkButtonWithLabel("", "How to fix this")
		helpLink.SetVisible(false)
		content.Append(helpLink)
		homeContent := gtk.NewBox(gtk.OrientationVertical, 12)
		homeContent.SetMarginTop(16)
		homeContent.SetMarginBottom(24)
		homeContent.SetMarginStart(24)
		homeContent.SetMarginEnd(24)
		homeIcon := gtk.NewImageFromIconName("com.tryomarchy.TryOmarchy")
		homeIcon.SetPixelSize(48)
		homeIcon.SetHAlign(gtk.AlignCenter)
		named(homeIcon, "Try Omarchy")
		homeContent.Append(homeIcon)
		homeTitle := gtk.NewLabel("Try Omarchy")
		homeTitle.AddCSSClass("title-1")
		homeContent.Append(homeTitle)
		homeHeadline := gtk.NewLabel("")
		homeHeadline.AddCSSClass("title-3")
		homeHeadline.SetWrap(true)
		homeHeadline.SetMaxWidthChars(48)
		homeHeadline.SetJustify(gtk.JustifyCenter)
		homeContent.Append(homeHeadline)
		homeStatus := gtk.NewLabel("")
		homeStatus.SetWrap(true)
		homeStatus.SetMaxWidthChars(48)
		homeStatus.SetJustify(gtk.JustifyCenter)
		homeContent.Append(homeStatus)
		homeDetail := gtk.NewLabel("")
		homeDetail.SetWrap(true)
		homeDetail.SetWrapMode(pango.WrapWordChar)
		homeDetail.SetSelectable(true)
		homeDetail.SetMaxWidthChars(48)
		homeDetail.SetJustify(gtk.JustifyCenter)
		homeDetail.AddCSSClass("dim-label")
		homeContent.Append(homeDetail)
		homeSections := gtk.NewBox(gtk.OrientationVertical, 12)
		homeDetails := gtk.NewExpander("Storage and account details")
		homeDetails.SetChild(homeSections)
		homeContent.Append(homeDetails)
		homeClamp := adw.NewClamp()
		homeClamp.SetMaximumSize(480)
		homeClamp.SetChild(homeContent)
		homeScroll := gtk.NewScrolledWindow()
		homeScroll.SetVExpand(true)
		homeScroll.SetChild(homeClamp)
		homeScroll.SetVisible(false)
		settingsContent := gtk.NewBox(gtk.OrientationVertical, 12)
		settingsContent.SetMarginTop(24)
		settingsContent.SetMarginBottom(24)
		settingsContent.SetMarginStart(24)
		settingsContent.SetMarginEnd(24)
		settingsTitle := gtk.NewLabel("Settings")
		settingsTitle.AddCSSClass("title-1")
		settingsContent.Append(settingsTitle)
		settingsStatus := gtk.NewLabel("")
		settingsStatus.SetWrap(true)
		settingsStatus.SetHAlign(gtk.AlignStart)
		settingsContent.Append(settingsStatus)
		form := gtk.NewBox(gtk.OrientationVertical, 8)
		// Headings stand apart from the controls above them, and help text
		// reads as secondary, so the long form scans as separate groups.
		formLabel := func(title string) {
			l := gtk.NewLabel(title)
			l.SetWrap(true)
			l.SetXAlign(0)
			l.AddCSSClass("heading")
			if form.FirstChild() != nil {
				l.SetMarginTop(12)
			}
			form.Append(l)
		}
		formHelp := func(text string) *gtk.Label {
			l := gtk.NewLabel(text)
			l.SetWrap(true)
			l.SetXAlign(0)
			l.AddCSSClass("dim-label")
			form.Append(l)
			return l
		}
		entry := func(title string) *gtk.Entry {
			formLabel(title)
			input := gtk.NewEntry()
			form.Append(input)
			return input
		}
		formLabel("Memory for Omarchy (GiB)")
		autoMemory := gtk.NewCheckButtonWithLabel("Choose memory automatically")
		memory := gtk.NewSpinButtonWithRange(1, 64, 0.25)
		memory.SetDigits(2)
		memory.SetNumeric(true)
		named(memory, "Memory for Omarchy in GiB")
		form.Append(autoMemory)
		form.Append(memory)
		autoMemory.ConnectToggled(func() { memory.SetSensitive(!autoMemory.Active()) })
		formLabel("Processors")
		autoCPUs := gtk.NewCheckButtonWithLabel("Choose processors automatically")
		cpus := gtk.NewSpinButtonWithRange(1, 64, 1)
		cpus.SetNumeric(true)
		named(cpus, "Processors for Omarchy")
		form.Append(autoCPUs)
		form.Append(cpus)
		autoCPUs.ConnectToggled(func() { cpus.SetSensitive(!autoCPUs.Active()) })
		formLabel("Rendering")
		render := gtk.NewDropDownFromStrings([]string{"Automatic (recommended)", "Graphics acceleration", "Software rendering"})
		named(render, "Rendering")
		form.Append(render)
		formHelp("Automatic tries graphics acceleration and falls back if needed. Changes take effect when the VM next starts.")
		defaults := gtk.NewButtonWithLabel("Restore resource defaults")
		defaults.ConnectClicked(func() {
			autoMemory.SetActive(true)
			autoCPUs.SetActive(true)
			render.SetSelected(0)
		})
		form.Append(defaults)
		formLabel("Disk capacity (GiB)")
		standardDisk := gtk.NewCheckButtonWithLabel("Use standard capacity (24 GiB)")
		diskGiB := gtk.NewSpinButtonWithRange(24, 1024, 1)
		diskGiB.SetNumeric(true)
		named(diskGiB, "Disk capacity in GiB")
		form.Append(standardDisk)
		form.Append(diskGiB)
		standardDisk.ConnectToggled(func() { diskGiB.SetSensitive(!standardDisk.Active()) })
		formHelp("A larger capacity grows the disk on the next launch. Existing disks are never shrunk.")
		formLabel("Display and audio")
		fullscreen := gtk.NewCheckButtonWithLabel("Open Omarchy fullscreen")
		form.Append(fullscreen)
		formLabel("Guest display scale")
		scaleChoices := []audioDevice{{"keep", "Keep guest choice"}, {"1", "100%"}, {"1.25", "125%"}, {"1.5", "150%"}, {"2", "200%"}, {"3", "300%"}, {"4", "400%"}}
		scale := gtk.NewDropDownFromStrings([]string{"Follow host display"})
		named(scale, "Guest display scale")
		form.Append(scale)
		scaleNames := []string{"auto"}
		formLabel("Guest keyboard layout")
		keyboardChoices := []audioDevice{{"keep", "Keep guest choice"}, {"us", "English (US)"}, {"us:intl", "English (US, international)"}, {"de", "German"}, {"fr", "French"}, {"es", "Spanish"}}
		keyboard := gtk.NewDropDownFromStrings([]string{"Follow host layout"})
		named(keyboard, "Guest keyboard layout")
		form.Append(keyboard)
		keyboardNames := []string{""}
		formHelp("Host layout changes while Omarchy runs apply on its next launch. Press Ctrl+Alt+G to release keyboard capture.")
		microphone := gtk.NewCheckButtonWithLabel("Allow microphone access")
		form.Append(microphone)
		camera := gtk.NewCheckButtonWithLabel("Allow camera access")
		form.Append(camera)
		formLabel("Camera")
		cameraChoice := gtk.NewDropDownFromStrings([]string{"Automatic"})
		named(cameraChoice, "Camera")
		form.Append(cameraChoice)
		cameraNames := []string{""}
		formHelp("Your desktop asks for camera permission when an app inside Omarchy first opens the camera. Capture stops when that app closes it. Access changes apply after shutting down Omarchy and launching it again.")
		formLabel("Audio output")
		audioOutput := gtk.NewDropDownFromStrings([]string{"System default"})
		named(audioOutput, "Audio output")
		form.Append(audioOutput)
		formLabel("Audio input")
		audioInput := gtk.NewDropDownFromStrings([]string{"System default"})
		named(audioInput, "Audio input")
		form.Append(audioInput)
		audioOutputNames, audioInputNames := []string{""}, []string{""}
		audioRefresh := gtk.NewButtonWithLabel("Refresh devices")
		form.Append(audioRefresh)
		audioHelp := formHelp("Audio device choices apply when the VM next starts. System default follows your desktop's current device.")
		clipboardShare := gtk.NewCheckButtonWithLabel("Share the clipboard with Omarchy")
		form.Append(clipboardShare)
		clipboardHelp := formHelp("GNOME asks for your permission the first time. A change applies the next time Omarchy starts.")
		formLabel("Network and SSH")
		sshEnabled := gtk.NewCheckButtonWithLabel("Allow SSH from this computer")
		form.Append(sshEnabled)
		sshPort := gtk.NewSpinButtonWithRange(1024, 65535, 1)
		sshPort.SetNumeric(true)
		named(sshPort, "SSH port on this computer")
		form.Append(sshPort)
		sshEnabled.ConnectToggled(func() { sshPort.SetSensitive(sshEnabled.Active()) })
		formHelp("SSH starts on the next launch. Connect to 127.0.0.1 on this port with your Omarchy account. Other computers cannot connect.")
		sshKey := entry("SSH public key (optional)")
		sshKey.SetEditable(false)
		named(sshKey, "SSH public key file")
		chooseSSHKey := gtk.NewButtonWithLabel("Choose a public key...")
		chooseSSHKey.ConnectClicked(func() {
			dialog := gtk.NewFileDialog()
			dialog.SetTitle("Choose an SSH public key")
			dialog.Open(context.Background(), &window.Window, func(result gio.AsyncResulter) {
				file, err := dialog.OpenFinish(result)
				if err != nil {
					detail, known := err.(interface {
						Quark() uint32
						ErrorCode() int
					})
					if !known || detail.Quark() != uint32(gtk.DialogErrorQuark()) || detail.ErrorCode() != int(gtk.DialogErrorDismissed) {
						settingsStatus.SetText("Could not choose that public key: " + err.Error())
					}
					return
				}
				if file.Path() == "" {
					settingsStatus.SetText("Choose a public key file on a local drive.")
					return
				}
				sshKey.SetText(file.Path())
			})
		})
		form.Append(chooseSSHKey)
		clearSSHKey := gtk.NewButtonWithLabel("Remove public key choice")
		clearSSHKey.ConnectClicked(func() { sshKey.SetText("") })
		form.Append(clearSSHKey)
		formLabel("Other local port forwards (one per line, for example tcp:8080:80)")
		forwards := gtk.NewTextView()
		forwards.SetSizeRequest(-1, 88)
		named(forwards, "Other local port forwards, one per line")
		form.Append(forwards)
		formLabel("Startup")
		startAutomatically := gtk.NewCheckButtonWithLabel("Start Omarchy when I open Try Omarchy")
		form.Append(startAutomatically)
		formHelp("The launcher stays open for 10 seconds so you can choose Settings or Close.")
		share := entry("Shared folder")
		share.SetEditable(false)
		named(share, "Shared folder")
		shareEnabled := gtk.NewCheckButtonWithLabel("Share this folder with Omarchy")
		form.Append(shareEnabled)
		clearShare := gtk.NewButtonWithLabel("Stop sharing this folder")
		clearShare.ConnectClicked(func() { share.SetText(""); shareEnabled.SetActive(false) })
		form.Append(clearShare)
		settingsContent.Append(form)
		settingsClamp := adw.NewClamp()
		settingsClamp.SetMaximumSize(480)
		settingsClamp.SetChild(settingsContent)
		settingsScroll := gtk.NewScrolledWindow()
		settingsScroll.SetVExpand(true)
		settingsScroll.SetChild(settingsClamp)
		settingsScroll.SetVisible(false)
		button := gtk.NewButtonWithLabel("Cancel")
		button.SetHAlign(gtk.AlignCenter)
		choices := gtk.NewBox(gtk.OrientationVertical, 8)
		primary := gtk.NewButton()
		primary.AddCSSClass("suggested-action")
		secondary := gtk.NewButton()
		tertiary := gtk.NewButtonWithLabel("Create diagnostics")
		moveButton := gtk.NewButtonWithLabel("Move this VM...")
		resetButton := gtk.NewButtonWithLabel("Reset this VM...")
		cleanMoveButton := gtk.NewButtonWithLabel("Remove previous copy...")
		cleanResetButton := gtk.NewButtonWithLabel("Remove disk kept from reset...")
		cleanLeftoversButton := gtk.NewButtonWithLabel("Remove unfinished files...")
		choices.Append(primary)
		choices.Append(secondary)
		choices.Append(tertiary)
		choices.Append(moveButton)
		choices.Append(resetButton)
		choices.Append(cleanMoveButton)
		choices.Append(cleanResetButton)
		choices.Append(cleanLeftoversButton)
		choices.SetVisible(false)
		// The home and error pages get exactly the buttons the launcher asked for.
		dynamicActions := gtk.NewBox(gtk.OrientationVertical, 8)
		dynamicActions.SetVisible(false)
		actions := gtk.NewBox(gtk.OrientationVertical, 8)
		actions.SetMarginTop(12)
		actions.SetMarginStart(24)
		actions.SetMarginEnd(24)
		actions.SetMarginBottom(16)
		actions.Append(dynamicActions)
		actions.Append(choices)
		actions.Append(button)
		clamp := adw.NewClamp()
		clamp.SetMaximumSize(480)
		clamp.SetTighteningThreshold(480)
		clamp.SetChild(content)
		page.SetChild(clamp)
		scroll := gtk.NewScrolledWindow()
		scroll.SetVExpand(true)
		scroll.SetChild(page)
		layout.Append(scroll)
		layout.Append(homeScroll)
		layout.Append(settingsScroll)
		// When the page scrolls, a rule marks where it ends above the pinned
		// buttons, so cut-off text does not look like it runs into them. It
		// fades rather than hides: this runs during layout, and showing or
		// hiding a widget there left the buttons below it undrawn.
		footerRule := gtk.NewSeparator(gtk.OrientationHorizontal)
		footerRule.SetOpacity(0)
		layout.Append(footerRule)
		updateFooterRule := func() {
			overflow := false
			for _, pane := range []*gtk.ScrolledWindow{scroll, homeScroll, settingsScroll} {
				adjustment := pane.VAdjustment()
				if pane.Visible() && adjustment.Upper() > adjustment.PageSize()+1 {
					overflow = true
				}
			}
			if overflow {
				footerRule.SetOpacity(1)
			} else {
				footerRule.SetOpacity(0)
			}
		}
		for _, pane := range []*gtk.ScrolledWindow{scroll, homeScroll, settingsScroll} {
			pane.VAdjustment().ConnectChanged(updateFooterRule)
			pane.NotifyProperty("visible", updateFooterRule)
		}
		actionClamp := adw.NewClamp()
		actionClamp.SetMaximumSize(480)
		actionClamp.SetChild(actions)
		layout.Append(actionClamp)
		window.SetContent(layout)
		// Recovery has more actions than fit under its storage summary in a
		// small window, so there they scroll with the page instead of staying
		// pinned above Back.
		choicesInPage := false
		placeChoices := func(inPage bool) {
			if inPage == choicesInPage {
				return
			}
			if inPage {
				actions.Remove(choices)
				content.Append(choices)
			} else {
				content.Remove(choices)
				actions.InsertChildAfter(choices, dynamicActions)
			}
			choicesInPage = inPage
		}
		failed, cancelling, determinate := false, false, false
		var current state
		answered := false
		reply := func(value string) {
			if current.Request == 0 || answered {
				return
			}
			answered = true
			choices.SetSensitive(false)
			dynamicActions.SetSensitive(false)
			menuButton.SetSensitive(false)
			if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"event": "reply", "request": current.Request, "value": value}); err != nil {
				app.Quit()
			}
		}
		emit := func(event string) {
			if err := json.NewEncoder(os.Stdout).Encode(map[string]string{"event": event}); err != nil {
				app.Quit()
			}
		}
		close := func() {
			if current.NonCancellable {
				return
			}
			if r := escapeReply(current); r != "" {
				reply(r)
			} else if failed {
				emit("dismissed")
				app.Quit()
			} else if !cancelling {
				cancelling = true
				if current.Booting {
					label.SetText("Asking Omarchy to shut down...")
				} else {
					label.SetText("Cancelling setup...")
				}
				button.SetSensitive(false)
				emit("cancel")
			}
		}
		// Escape closes a prompt the way its Back or Cancel button does. Progress
		// screens ignore it so a stray key never stops a download.
		keys := gtk.NewEventControllerKey()
		keys.ConnectKeyPressed(func(keyval, keycode uint, mods gdk.ModifierType) bool {
			if keyval == gdk.KEY_Escape && escapeReply(current) != "" {
				close()
				return true
			}
			return false
		})
		window.AddController(keys)
		settingsValue := func(refresh bool) string {
			memoryValue, cpuValue := "0", "0"
			if !autoMemory.Active() {
				memoryValue = strconv.Itoa(int(memory.Value()*1024 + 0.5))
			}
			if !autoCPUs.Active() {
				cpuValue = strconv.Itoa(cpus.ValueAsInt())
			}
			modes := []string{"auto", "gpu", "cpu"}
			diskValue := "0"
			if !standardDisk.Active() {
				diskValue = strconv.Itoa(diskGiB.ValueAsInt())
			}
			forwardStart, forwardEnd := forwards.Buffer().Bounds()
			forwardText := forwards.Buffer().Text(forwardStart, forwardEnd, false)
			data, _ := json.Marshal(settingsForm{RefreshAudio: refresh, Memory: memoryValue, CPUs: cpuValue, Render: modes[min(int(render.Selected()), 2)], Fullscreen: fullscreen.Active(), Microphone: microphone.Active(), Camera: camera.Active(), CameraID: cameraNames[min(int(cameraChoice.Selected()), len(cameraNames)-1)], AudioOutput: audioOutputNames[min(int(audioOutput.Selected()), len(audioOutputNames)-1)], AudioInput: audioInputNames[min(int(audioInput.Selected()), len(audioInputNames)-1)], DiskGiB: diskValue, Scale: scaleNames[min(int(scale.Selected()), len(scaleNames)-1)], Keyboard: keyboardNames[min(int(keyboard.Selected()), len(keyboardNames)-1)], SSHEnabled: sshEnabled.Active(), SSHPort: strconv.Itoa(sshPort.ValueAsInt()), SSHKey: sshKey.Text(), Forwards: forwardText, StartAutomatically: startAutomatically.Active(), Share: share.Text(), ShareEnabled: shareEnabled.Active(), CPUMax: current.Settings.CPUMax, ClipboardShare: clipboardShare.Active(), ClipboardAvailable: current.Settings.ClipboardAvailable})
			return string(data)
		}
		audioRefresh.ConnectClicked(func() {
			if current.Prompt == "settings" {
				reply(settingsValue(true))
			}
		})
		primary.ConnectClicked(func() {
			switch current.Prompt {
			case "about", "settings-saved":
				reply("back")
			case "location":
				reply("default")
			case "account":
				reply("personal")
			case "close":
				reply("keep")
			case "forget-location":
				reply("keep")
			case "delete-default":
				reply("keep")
			case "share":
				reply("skip")
			case "recovery":
				reply("backup")
			case "choice":
				reply("primary")
			case "error":
				reply("retry")
			case "backup-folder", "restore-parent", "restore-archive", "attach-folder", "move-folder":
				id := current.Request
				dialog := gtk.NewFileDialog()
				if current.Prompt == "restore-archive" {
					dialog.SetTitle("Choose a Try Omarchy backup")
				} else if current.Prompt == "attach-folder" {
					dialog.SetTitle("Choose existing Try Omarchy data folder")
				} else if current.Prompt == "move-folder" {
					dialog.SetTitle("Choose where to move Omarchy")
				} else {
					dialog.SetTitle("Choose a recovery folder")
				}
				choices.SetSensitive(false)
				selected := func(path string, err error) {
					if cancelling || answered || id != current.Request {
						return
					}
					choices.SetSensitive(true)
					if err != nil || path == "" {
						label.SetText("No location was chosen. Choose one or cancel.")
						return
					}
					reply(path)
				}
				if current.Prompt == "restore-archive" {
					dialog.Open(context.Background(), &window.Window, func(result gio.AsyncResulter) {
						file, err := dialog.OpenFinish(result)
						if err != nil {
							selected("", err)
						} else {
							selected(file.Path(), nil)
						}
					})
				} else {
					dialog.SelectFolder(context.Background(), &window.Window, func(result gio.AsyncResulter) {
						folder, err := dialog.SelectFolderFinish(result)
						if err != nil {
							selected("", err)
						} else {
							selected(folder.Path(), nil)
						}
					})
				}
			case "grant-files":
				id := current.Request
				dialog := gtk.NewFileDialog()
				dialog.SetTitle("Choose the dropped files")
				dialog.SetAcceptLabel("Share with Omarchy")
				choices.SetSensitive(false)
				dialog.OpenMultiple(context.Background(), &window.Window, func(result gio.AsyncResulter) {
					files, err := dialog.OpenMultipleFinish(result)
					if cancelling || answered || id != current.Request {
						return
					}
					choices.SetSensitive(true)
					if err != nil {
						label.SetText("File access was not granted. Choose files or cancel.")
						return
					}
					var paths []string
					for i := uint(0); i < files.NItems(); i++ {
						if object := files.Item(i); object != nil {
							file := &gio.File{Object: object}
							if path := file.Path(); path != "" {
								paths = append(paths, path)
							}
						}
					}
					data, _ := json.Marshal(paths)
					reply(string(data))
				})
			case "settings":
				reply(settingsValue(false))
			}
		})
		secondary.ConnectClicked(func() {
			switch current.Prompt {
			case "account":
				reply("instant")
			case "close":
				reply("shutdown")
			case "forget-location":
				reply("forget")
			case "delete-default":
				reply("delete")
			case "recovery":
				reply("restore")
			case "choice":
				reply("secondary")
			case "location", "share", "settings":
				id := current.Request
				dialog := gtk.NewFileDialog()
				dialog.SetTitle("Choose a folder for Omarchy")
				if current.Prompt == "share" || current.Prompt == "settings" {
					dialog.SetTitle("Choose a shared folder")
				}
				dialog.SetAcceptLabel("Use this folder")
				choices.SetSensitive(false)
				dialog.SelectFolder(context.Background(), &window.Window, func(result gio.AsyncResulter) {
					folder, err := dialog.SelectFolderFinish(result)
					if cancelling || answered || id != current.Request {
						return
					}
					choices.SetSensitive(true)
					if err != nil {
						detail, known := err.(interface {
							Quark() uint32
							ErrorCode() int
						})
						if !known || detail.Quark() != uint32(gtk.DialogErrorQuark()) || detail.ErrorCode() != int(gtk.DialogErrorDismissed) {
							label.SetText("Could not choose that folder: " + err.Error())
							fmt.Fprintln(os.Stderr, "Folder selection:", err)
						}
						return
					}
					if folder.Path() == "" {
						label.SetText("Choose a folder on a local drive.")
						return
					}
					if current.Prompt == "settings" {
						share.SetText(folder.Path())
						shareEnabled.SetActive(true)
					} else {
						reply(folder.Path())
					}
				})
			}
		})
		tertiary.ConnectClicked(func() {
			if current.Prompt == "recovery" {
				reply("diagnostics")
			}
		})
		moveButton.ConnectClicked(func() {
			if current.Prompt == "recovery" && current.CanMove {
				reply("move")
			}
		})
		resetButton.ConnectClicked(func() {
			if current.Prompt == "recovery" && current.CanReset {
				reply("reset")
			}
		})
		cleanMoveButton.ConnectClicked(func() {
			if current.Prompt == "recovery" && current.CanCleanMove {
				reply("clean-move")
			}
		})
		cleanResetButton.ConnectClicked(func() {
			if current.Prompt == "recovery" && current.CanCleanReset {
				reply("clean-reset")
			}
		})
		cleanLeftoversButton.ConnectClicked(func() {
			if current.Prompt == "recovery" && current.CanCleanLeftovers {
				reply("clean-leftovers")
			}
		})
		button.ConnectClicked(close)
		window.ConnectCloseRequest(func() bool { close(); return true })
		glib.TimeoutAdd(100, func() bool {
			if !determinate && !failed && current.Prompt == "" {
				progress.Pulse()
			}
			return true
		})
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Buffer(make([]byte, 4096), 1024*1024)
			for scanner.Scan() {
				var next state
				if err := json.Unmarshal(scanner.Bytes(), &next); err != nil {
					fmt.Fprintln(os.Stderr, "Invalid setup message:", err)
					break
				}
				glib.IdleAdd(func() {
					if !acceptStateAfterCancel(cancelling, next) {
						return
					}
					cancelling = false
					current = next
					answered = false
					choices.SetSensitive(true)
					dynamicActions.SetSensitive(true)
					menuButton.SetSensitive(true)
					button.SetSensitive(true)
					window.SetDeletable(!next.NonCancellable)
					// The banner carries a problem the user can fix. Error pages
					// carry their own message, so they never show it twice.
					bannerURL = next.HelpURL
					banner.SetTitle(next.Notice)
					banner.SetButtonLabel("")
					if next.HelpURL != "" {
						banner.SetButtonLabel("How to fix this")
					}
					banner.SetRevealed(next.Notice != "" && !next.Error && next.Prompt != "error")
					// The home and error pages get the buttons the launcher chose.
					dynamic := next.Prompt == "home" || next.Prompt == "error"
					choices.SetVisible(next.Prompt != "" && !dynamic)
					placeChoices(next.Prompt == "recovery")
					dynamicActions.SetVisible(dynamic)
					button.SetVisible(!dynamic && !next.NonCancellable && next.Prompt != "close" && next.Prompt != "about" && next.Prompt != "settings-saved")
					tertiary.SetVisible(next.Prompt == "recovery")
					moveButton.SetVisible(next.Prompt == "recovery" && next.CanMove)
					resetButton.SetVisible(next.Prompt == "recovery" && next.CanReset)
					cleanMoveButton.SetVisible(next.Prompt == "recovery" && next.CanCleanMove)
					cleanResetButton.SetVisible(next.Prompt == "recovery" && next.CanCleanReset)
					cleanLeftoversButton.SetVisible(next.Prompt == "recovery" && next.CanCleanLeftovers)
					tertiary.SetLabel("Create diagnostics")
					button.SetLabel("Cancel")
					if next.Booting && next.Prompt == "" {
						button.SetLabel("Stop Omarchy")
					}
					page.SetTitle("Try Omarchy")
					page.SetIconName("com.tryomarchy.TryOmarchy")
					if next.Prompt != "" {
						page.AddCSSClass("compact")
						if len(next.Sections) > 0 {
							page.SetIconName("")
						}
					} else {
						page.RemoveCSSClass("compact")
					}
					scroll.SetVisible(next.Prompt != "home" && next.Prompt != "settings")
					homeScroll.SetVisible(next.Prompt == "home")
					settingsScroll.SetVisible(next.Prompt == "settings")
					homeHeadline.SetText(next.Headline)
					homeHeadline.SetVisible(next.Headline != "")
					homeStatus.SetText(next.Status)
					homeDetail.SetText(next.Detail)
					homeDetail.SetVisible(next.Detail != "")
					settingsStatus.SetText(next.Status)
					secondary.RemoveCSSClass("destructive-action")
					fillSections(homeSections, sectionsIf(next.Prompt == "home", next.Sections))
					homeDetails.SetVisible(next.Prompt == "home" && len(next.Sections) > 0)
					homeDetails.SetExpanded(next.Installed)
					fillSections(pageSections, sectionsIf(next.Prompt != "home" && next.Prompt != "settings", next.Sections))
					clearChildren(menuBox)
					for _, item := range next.Menu {
						item := item
						entry := gtk.NewButtonWithLabel(item.Label)
						entry.AddCSSClass("flat")
						if item.Destructive {
							entry.AddCSSClass("error")
						}
						entry.ConnectClicked(func() {
							menuPopover.Popdown()
							reply(item.Reply)
						})
						menuBox.Append(entry)
					}
					menuButton.SetVisible(next.Prompt == "home" && len(next.Menu) > 0)
					switch next.Prompt {
					case "recovery":
						page.SetTitle("Backup and recovery")
						page.SetIconName("")
						primary.SetLabel("Create backup")
						secondary.SetLabel("Restore as a copy")
						button.SetLabel("Back")
					case "forget-location":
						page.SetTitle("Forget saved location?")
						primary.SetLabel("Keep saved location")
						secondary.SetLabel("Forget saved location")
						secondary.AddCSSClass("destructive-action")
						button.SetLabel("Cancel")
					case "delete-default":
						page.SetTitle("Delete this VM?")
						primary.SetLabel("Keep this VM")
						secondary.SetLabel("Delete VM and system files")
						secondary.AddCSSClass("destructive-action")
						button.SetLabel("Cancel")
					case "backup-folder", "restore-parent", "restore-archive", "attach-folder", "move-folder":
						page.SetTitle("Backup and recovery")
						if next.Prompt == "attach-folder" {
							page.SetTitle("Use an existing VM")
						}
						if next.Prompt == "move-folder" {
							page.SetTitle("Move this VM")
						}
						primary.SetLabel("Choose location...")
						button.SetLabel("Cancel")
					case "choice":
						page.SetTitle(next.Title)
						primary.SetLabel(next.Primary)
						secondary.SetLabel(next.Secondary)
						if next.Destructive {
							secondary.AddCSSClass("destructive-action")
						}
						button.SetLabel("Cancel")
					case "about":
						page.SetTitle("About Try Omarchy")
						primary.SetLabel("Back")
						button.SetLabel("Back")
						if next.Status == "" {
							next.Status = "Try Omarchy for Linux, version " + next.Version + "."
						}
					case "settings-saved":
						page.SetTitle("Settings saved")
						primary.SetLabel("Done")
						button.SetLabel("Close")
					case "settings":
						page.SetTitle("Settings")
						page.SetIconName("")
						primary.SetLabel("Save settings")
						secondary.SetLabel("Choose a shared folder...")
						if next.Settings != nil {
							memoryMiB, _ := strconv.Atoi(next.Settings.Memory)
							cpuCount, _ := strconv.Atoi(next.Settings.CPUs)
							autoMemory.SetActive(memoryMiB == 0)
							autoCPUs.SetActive(cpuCount == 0)
							memory.SetValue(float64(max(memoryMiB, 1024)) / 1024)
							cpus.SetRange(1, float64(max(next.Settings.CPUMax, 1)))
							cpus.SetValue(float64(max(cpuCount, 1)))
							switch next.Settings.Render {
							case "gpu":
								render.SetSelected(1)
							case "cpu":
								render.SetSelected(2)
							default:
								render.SetSelected(0)
							}
							fullscreen.SetActive(next.Settings.Fullscreen)
							microphone.SetActive(next.Settings.Microphone)
							camera.SetActive(next.Settings.Camera)
							cameraLabels, names, index := namedChoices("Automatic", "Unavailable: ", next.Settings.Cameras, next.Settings.CameraID)
							cameraNames = names
							cameraChoice.SetModel(gtk.NewStringList(cameraLabels))
							cameraChoice.SetSelected(index)
							diskCapacity, _ := strconv.Atoi(next.Settings.DiskGiB)
							standardDisk.SetActive(diskCapacity == 0)
							diskGiB.SetValue(float64(max(diskCapacity, 24)))
							selectedScale := next.Settings.Scale
							if selectedScale == "auto" {
								selectedScale = ""
							}
							scaleLabels, selectedScales, scaleIndex := namedChoices("Follow host display", "Saved scale: ", scaleChoices, selectedScale)
							scaleNames = selectedScales
							scaleNames[0] = "auto"
							scale.SetModel(gtk.NewStringList(scaleLabels))
							scale.SetSelected(scaleIndex)
							keyboardLabels, selectedKeyboards, keyboardIndex := namedChoices("Follow host layout", "Saved layout: ", keyboardChoices, next.Settings.Keyboard)
							keyboardNames = selectedKeyboards
							keyboard.SetModel(gtk.NewStringList(keyboardLabels))
							keyboard.SetSelected(keyboardIndex)
							sshEnabled.SetActive(next.Settings.SSHEnabled)
							sshPortValue, _ := strconv.Atoi(next.Settings.SSHPort)
							sshPort.SetValue(float64(max(sshPortValue, 1024)))
							sshKey.SetText(next.Settings.SSHKey)
							forwards.Buffer().SetText(next.Settings.Forwards)
							startAutomatically.SetActive(next.Settings.StartAutomatically)
							outputLabels, outputNames, outputIndex := namedChoices("System default", "Unavailable: ", next.Settings.AudioOutputs, next.Settings.AudioOutput)
							if next.Settings.AudioLive {
								audioHelp.SetText("Audio device choices apply when you save. Microphone access changes apply on the next launch. System default follows your desktop's current device.")
							} else {
								audioHelp.SetText("Audio device choices apply when the VM next starts. System default follows your desktop's current device.")
							}
							audioOutputNames = outputNames
							audioOutput.SetModel(gtk.NewStringList(outputLabels))
							audioOutput.SetSelected(outputIndex)
							inputLabels, inputNames, inputIndex := namedChoices("System default", "Unavailable: ", next.Settings.AudioInputs, next.Settings.AudioInput)
							audioInputNames = inputNames
							audioInput.SetModel(gtk.NewStringList(inputLabels))
							audioInput.SetSelected(inputIndex)
							clipboardShare.SetActive(next.Settings.ClipboardShare)
							clipboardShare.SetVisible(next.Settings.ClipboardAvailable)
							clipboardHelp.SetVisible(next.Settings.ClipboardAvailable)
							share.SetText(next.Settings.Share)
							shareEnabled.SetActive(next.Settings.ShareEnabled)
						}
					case "location":
						page.SetTitle("Where should Omarchy live?")
						primary.SetLabel("Use default location")
						secondary.SetLabel("Choose another folder...")
					case "account":
						page.SetTitle("Make yourself at home")
						primary.SetLabel("Set up my own account")
						secondary.SetLabel("Quick start as omarchy")
					case "share":
						page.SetTitle("Share a folder with Omarchy?")
						primary.SetLabel("Not now")
						secondary.SetLabel("Choose a shared folder...")
						if next.Status == "" {
							next.Status = "Omarchy can read, change, and delete files in the folder you choose. Use a dedicated folder to exchange files."
						}
					case "grant-files":
						page.SetTitle("Share dropped files")
						primary.SetLabel("Choose files...")
						button.SetLabel("Cancel")
					case "close":
						page.SetTitle("Shut down Omarchy?")
						primary.SetLabel("Keep running")
						secondary.SetLabel("Shut down")
						secondary.AddCSSClass("destructive-action")
						if next.Status == "" {
							next.Status = "Save your work inside Omarchy before shutting down."
						}
					case "error":
						page.SetTitle(orDefault(next.ErrorTitle, "Omarchy could not start"))
						page.SetIconName("dialog-error-symbolic")
					}
					secondary.SetVisible(!oneButtonPrompt(next.Prompt) && next.Prompt != "error")
					// The home's buttons, or an error page's, in the order asked for.
					dynamicList := next.Actions
					if next.Prompt == "error" {
						dynamicList = errorActions(next)
					}
					var focus *gtk.Button
					clearChildren(dynamicActions)
					for i, item := range dynamicList {
						item := item
						b := gtk.NewButtonWithLabel(item.Label)
						if item.Suggested {
							b.AddCSSClass("suggested-action")
							b.AddCSSClass("pill")
						}
						if item.Destructive {
							b.AddCSSClass("destructive-action")
						}
						if item.Reply == "close" {
							b.AddCSSClass("flat")
						}
						b.ConnectClicked(func() { reply(item.Reply) })
						dynamicActions.Append(b)
						if i == homeSuggested(dynamicList) {
							focus = b
						}
					}
					if focus != nil {
						focus.GrabFocus()
					} else if next.Prompt != "" {
						primary.GrabFocus()
					}
					failed = next.Error
					label.SetText(next.Status)
					label.SetSelectable(failed || next.Prompt == "error")
					determinate = next.Total > 0
					progress.SetShowText(determinate)
					progress.SetVisible(!failed && next.Prompt == "")
					if determinate {
						progress.SetFraction(min(max(float64(next.Current)/float64(next.Total), 0), 1))
						progress.SetText(percentText(next.Current, next.Total))
					} else {
						progress.SetText("")
					}
					detail.SetText(next.Detail)
					detail.SetVisible(next.Detail != "" && next.Prompt != "home")
					helpLink.SetVisible((failed || next.Prompt == "error") && next.HelpURL != "")
					if next.HelpURL != "" {
						helpLink.SetURI(next.HelpURL)
					}
					if failed {
						page.SetTitle(orDefault(next.ErrorTitle, "Omarchy could not start"))
						page.SetIconName("dialog-error-symbolic")
						button.SetLabel("Close")
					}
					page.QueueResize()
				})
			}
			// EOF also closes the window when the launcher exits or crashes.
			glib.IdleAdd(app.Quit)
		}()
		window.Present()
		emit("ready")
	})
	os.Exit(app.Run(os.Args))
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func sectionsIf(show bool, sections []section) []section {
	if show {
		return sections
	}
	return nil
}

// oneButtonPrompt lists prompts whose only choice is Back, Close or a picker.
func oneButtonPrompt(prompt string) bool {
	switch prompt {
	case "about", "settings-saved", "grant-files", "backup-folder", "restore-parent", "restore-archive", "attach-folder", "move-folder":
		return true
	}
	return false
}
