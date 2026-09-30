// try-omarchy-setup owns only the setup window. The launcher owns downloads,
// disks and the VM, and sends complete UI snapshots as newline-delimited JSON.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

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
	Running            bool          `json:"running"`
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
	ResourceProfile    string        `json:"resourceProfile"`
	ResourceSummary    string        `json:"resourceSummary"`
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

// fillSections lays headed rows out as boxed lists. Names stay prominent and
// details can be selected so a path can be copied.
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
			iconName := "dialog-information-symbolic"
			switch {
			case strings.Contains(strings.ToLower(r.Title), "resource"):
				iconName = "preferences-system-symbolic"
			case strings.Contains(strings.ToLower(r.Title), "microphone"):
				iconName = "audio-input-microphone-symbolic"
			case strings.Contains(strings.ToLower(r.Title), "camera"):
				iconName = "camera-photo-symbolic"
			case strings.Contains(strings.ToLower(r.Title), "folder"), strings.Contains(strings.ToLower(r.Title), "location"):
				iconName = "folder-symbolic"
			case strings.Contains(strings.ToLower(r.Title), "disk"), strings.Contains(strings.ToLower(r.Title), "space"):
				iconName = "drive-harddisk-symbolic"
			}
			icon := gtk.NewImageFromIconName(iconName)
			icon.AddCSSClass("integration-icon")
			icon.SetPixelSize(20)
			item.AddPrefix(icon)
			if r.State != "" {
				text, style := "Next launch", "dim-label"
				switch r.State {
				case "enabled":
					text, style = "On", "success"
					if r.Title == "Microphone" || r.Title == "Camera" {
						text = "Allowed"
					}
				case "disabled":
					text = "Off"
				case "unavailable":
					text, style = "Unavailable", "warning"
				}
				status := gtk.NewLabel(text)
				status.AddCSSClass(style)
				status.AddCSSClass("integration-state")
				item.AddSuffix(status)
			}

			group.Add(item)
		}
		box.Append(group)
	}
	box.SetVisible(len(sections) > 0)
}

func main() {
	os.Exit(runUI(os.Stdin, os.Stdout, nil))
}

// Native controls may contain single-line labels whose minimum width grows
// with the desktop's text scale or a long device name. Let their text wrap
// instead of widening the scroll viewport past a small window.
func wrapWindowLabels(widget *gtk.Widget) {
	if label, ok := widget.Object.Cast().(*gtk.Label); ok {
		if label.HasCSSClass("integration-state") {
			label.SetWrap(false)
		} else {
			label.SetWrap(true)
			label.SetWrapMode(pango.WrapWordChar)
		}
	}
	for child := widget.FirstChild(); child != nil; child = gtk.BaseWidget(child).NextSibling() {
		wrapWindowLabels(gtk.BaseWidget(child))
	}
}

// runUI presents complete backend snapshots with native controls and writes
// request-scoped replies. onWindow lets isolated tests inspect the real window.
func runUI(input io.Reader, output io.Writer, onWindow func(*adw.ApplicationWindow)) int {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	glib.SetPrgname("com.tryomarchy.TryOmarchy")
	glib.SetApplicationName("Try Omarchy")
	app := adw.NewApplication("com.tryomarchy.TryOmarchy", gio.ApplicationNonUnique)
	app.ConnectActivate(func() {
		window := adw.NewApplicationWindow(&app.Application)
		var current state
		answered := false
		window.SetTitle("Try Omarchy")
		window.SetDefaultSize(640, 700)
		window.SetResizable(true)
		applyBrand(window)
		layout := gtk.NewBox(gtk.OrientationVertical, 0)
		header := adw.NewHeaderBar()
		header.AddCSSClass("flat")
		header.SetTitleWidget(gtk.NewLabel(""))
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
		productHeader, productVersion := newBrandHeader()
		productHeader.SetMarginTop(12)
		productHeader.SetMarginBottom(20)
		productHeader.SetMarginStart(24)
		productHeader.SetMarginEnd(24)
		productClamp := adw.NewClamp()
		productClamp.SetMaximumSize(640)
		productClamp.SetTighteningThreshold(640)
		productClamp.SetChild(productHeader)
		layout.Append(productClamp)
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
		showSettingsProblem := func(message string) {
			bannerURL = ""
			banner.SetButtonLabel("")
			banner.SetTitle(message)
			banner.SetRevealed(true)
		}
		page := gtk.NewBox(gtk.OrientationVertical, 0)
		page.SetVExpand(true)
		content := gtk.NewBox(gtk.OrientationVertical, 16)
		content.SetMarginTop(4)
		content.SetMarginStart(24)
		content.SetMarginEnd(24)
		content.SetMarginBottom(16)
		pageTitle := gtk.NewLabel("")
		pageTitle.AddCSSClass("title-2")
		pageTitle.AddCSSClass("heading")
		pageTitle.SetXAlign(0)
		content.Append(pageTitle)
		pageIcon := gtk.NewImageFromIconName("dialog-error-symbolic")
		pageIcon.SetPixelSize(32)
		pageIcon.SetHAlign(gtk.AlignStart)
		pageIcon.AddCSSClass("error")
		pageIcon.SetVisible(false)
		content.Append(pageIcon)
		label := gtk.NewLabel("Preparing Omarchy...")
		label.SetWrap(true)
		label.SetWrapMode(pango.WrapWordChar)
		label.SetMaxWidthChars(48)
		label.SetJustify(gtk.JustifyLeft)
		label.SetXAlign(0)
		content.Append(label)
		pageSections := gtk.NewBox(gtk.OrientationVertical, 12)
		pageSections.SetVisible(false)
		content.Append(pageSections)
		accountOptions := gtk.NewBox(gtk.OrientationVertical, 16)
		personalAccount := gtk.NewCheckButtonWithLabel("My own username and password (recommended)")
		quickAccount := gtk.NewCheckButtonWithLabel("Quick start (omarchy / omarchy)")
		quickAccount.SetGroup(personalAccount)
		personalAccount.SetActive(true)
		accountOptions.Append(personalAccount)
		accountOptions.Append(quickAccount)
		accountOptions.SetVisible(false)
		content.Append(accountOptions)
		progress := gtk.NewProgressBar()
		named(progress, "Progress")
		content.Append(progress)
		detail := gtk.NewLabel("")
		detail.SetWrap(true)
		detail.SetWrapMode(pango.WrapWordChar)
		detail.SetMaxWidthChars(48)
		detail.SetJustify(gtk.JustifyLeft)
		detail.SetXAlign(0)
		detail.AddCSSClass("dim-label")
		detail.SetVisible(false)
		content.Append(detail)
		helpLink := gtk.NewLinkButtonWithLabel("", "How to fix this")
		helpLink.SetVisible(false)
		content.Append(helpLink)
		homeContent := gtk.NewBox(gtk.OrientationVertical, 12)
		homeContent.SetMarginTop(4)
		homeContent.SetMarginBottom(24)
		homeContent.SetMarginStart(24)
		homeContent.SetMarginEnd(24)
		homeHeadline := gtk.NewLabel("")
		homeHeadline.AddCSSClass("title-3")
		homeHeadline.SetWrap(true)
		homeHeadline.SetMaxWidthChars(48)
		homeHeadline.SetJustify(gtk.JustifyLeft)
		homeHeadline.SetXAlign(0)
		homeContent.Append(homeHeadline)
		homeStatus := gtk.NewLabel("")
		homeStatus.SetWrap(true)
		homeStatus.SetMaxWidthChars(48)
		homeStatus.SetJustify(gtk.JustifyLeft)
		homeStatus.SetXAlign(0)
		homeContent.Append(homeStatus)
		homeDetail := gtk.NewLabel("")
		homeDetail.SetWrap(true)
		homeDetail.SetWrapMode(pango.WrapWordChar)
		homeDetail.SetSelectable(true)
		homeDetail.SetMaxWidthChars(48)
		homeDetail.SetJustify(gtk.JustifyLeft)
		homeDetail.SetXAlign(0)
		homeDetail.AddCSSClass("dim-label")
		homeContent.Append(homeDetail)
		homeSections := gtk.NewBox(gtk.OrientationVertical, 12)
		homeContent.Append(homeSections)
		homeSetup := gtk.NewBox(gtk.OrientationVertical, 12)
		homeDetails := gtk.NewExpander("What setup does")
		homeDetails.SetChild(homeSetup)
		homeContent.Append(homeDetails)
		homeClamp := adw.NewClamp()
		homeClamp.SetMaximumSize(640)
		homeClamp.SetTighteningThreshold(640)
		homeClamp.SetChild(homeContent)
		homeScroll := gtk.NewScrolledWindow()
		homeScroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
		homeScroll.SetVExpand(true)
		homeScroll.SetChild(homeClamp)
		homeScroll.SetVisible(false)
		settingsContent := gtk.NewBox(gtk.OrientationVertical, 16)
		settingsContent.SetMarginTop(4)
		settingsContent.SetMarginBottom(20)
		settingsContent.SetMarginStart(24)
		settingsContent.SetMarginEnd(24)
		settingsStatus := gtk.NewLabel("")
		settingsStatus.SetWrap(true)
		settingsStatus.SetXAlign(0)
		settingsStatus.AddCSSClass("dim-label")
		form := gtk.NewStack()
		form.SetHhomogeneous(false)
		form.SetVhomogeneous(false)
		form.AddCSSClass("settings-pages")
		named(form, "Settings pages")
		settingsTabs := gtk.NewStackSwitcher()
		settingsTabs.SetStack(form)
		settingsTabs.SetHAlign(gtk.AlignFill)
		settingsTabs.AddCSSClass("settings-tabs")
		settingsTabs.SetMarginStart(24)
		settingsTabs.SetMarginEnd(24)
		settingsTabs.SetMarginBottom(16)
		settingsTabs.SetVisible(false)
		layout.Append(settingsTabs)
		settingsPages := map[string]*gtk.Box{}
		for _, title := range []string{"General", "Devices", "Advanced"} {
			body := gtk.NewBox(gtk.OrientationVertical, 20)
			settingsPages[title] = body
			form.AddTitled(body, title, title)
		}
		settingsPages["Devices"].Append(settingsStatus)
		var groupContent *gtk.Box
		beginGroup := func(title, page string) *adw.PreferencesGroup {
			group := adw.NewPreferencesGroup()
			group.SetTitle(title)
			groupContent = gtk.NewBox(gtk.OrientationVertical, 10)
			groupContent.AddCSSClass("card")
			groupContent.AddCSSClass("settings-card")
			group.Add(groupContent)
			settingsPages[page].Append(group)
			return group
		}
		formLabel := func(title string) {
			l := gtk.NewLabel(title)
			l.SetWrap(true)
			l.SetXAlign(0)
			l.AddCSSClass("heading")
			groupContent.Append(l)
		}
		formHelp := func(text string) *gtk.Label {
			l := gtk.NewLabel(text)
			l.SetWrap(true)
			l.SetXAlign(0)
			l.AddCSSClass("dim-label")
			l.AddCSSClass("setting-help")
			groupContent.Append(l)
			return l
		}
		formField := func(title string, control gtk.Widgetter) *gtk.FlowBox {
			l := gtk.NewLabel(title)
			l.SetWrap(true)
			l.SetWrapMode(pango.WrapWordChar)
			l.SetXAlign(0)
			l.SetSizeRequest(160, -1)
			l.SetMnemonicWidget(control)
			gtk.BaseWidget(control).SetHExpand(true)
			row := newResponsiveRow(l, control)
			row.AddCSSClass("setting-field")
			groupContent.Append(row)
			return row
		}
		entry := func(title string) *gtk.Entry {
			input := gtk.NewEntry()
			formField(title, input)
			return input
		}
		beginGroup("Display and startup", "General")
		fullscreen := gtk.NewCheckButtonWithLabel("Open fullscreen")
		groupContent.Append(fullscreen)
		startAutomatically := gtk.NewCheckButtonWithLabel("Start Omarchy when I open Try Omarchy")
		groupContent.Append(startAutomatically)
		formHelp("Automatic start waits 10 seconds; Settings or Close stops it.")
		fullscreen.SetTooltipText("Ctrl+Alt+F toggles fullscreen. Ctrl+Alt+G releases the keyboard.")
		beginGroup("Resources", "General")
		resourceProfile := gtk.NewDropDownFromStrings([]string{"Balanced (recommended)", "Maximum performance", "Manual"})
		named(resourceProfile, "Resource profile")
		formField("Resource profile", resourceProfile)
		resourceSummary := formHelp("")
		automaticSummary := ""
		resourceCard := groupContent
		manualResources := gtk.NewBox(gtk.OrientationVertical, 8)
		resourceCard.Append(manualResources)
		updateResources := func() {
			manualResources.SetVisible(resourceProfile.Selected() == 2)
			switch resourceProfile.Selected() {
			case 1:
				resourceSummary.SetText("Uses available resources at the next launch while leaving room for Linux. The running VM is not resized.")
			case 2:
				resourceSummary.SetText("Choose memory and processors below. Saved manual values are retained when you use another profile.")
			default:
				resourceSummary.SetText(automaticSummary)
			}
		}
		resourceProfile.NotifyProperty("selected", updateResources)
		groupContent = manualResources
		formLabel("Memory for Omarchy (GiB)")
		autoMemory := gtk.NewCheckButtonWithLabel("Choose memory automatically")
		memory := gtk.NewSpinButtonWithRange(1, 64, 0.25)
		memory.SetDigits(2)
		memory.SetNumeric(true)
		named(memory, "Memory for Omarchy in GiB")
		groupContent.Append(autoMemory)
		groupContent.Append(memory)
		autoMemory.ConnectToggled(func() { memory.SetVisible(!autoMemory.Active()) })
		formLabel("Processors")
		autoCPUs := gtk.NewCheckButtonWithLabel("Choose processors automatically")
		cpus := gtk.NewSpinButtonWithRange(1, 64, 1)
		cpus.SetNumeric(true)
		named(cpus, "Processors for Omarchy")
		groupContent.Append(autoCPUs)
		groupContent.Append(cpus)
		autoCPUs.ConnectToggled(func() { cpus.SetVisible(!autoCPUs.Active()) })
		groupContent = resourceCard
		render := gtk.NewDropDownFromStrings([]string{"Automatic (recommended)", "Graphics acceleration", "Software rendering"})
		named(render, "Rendering")
		defaults := gtk.NewButtonWithLabel("Restore resource defaults")
		defaults.ConnectClicked(func() {
			resourceProfile.SetSelected(0)
			autoMemory.SetActive(true)
			autoCPUs.SetActive(true)
			render.SetSelected(0)
		})
		defaults.SetHAlign(gtk.AlignStart)
		beginGroup("Storage and sharing", "General")
		standardDisk := gtk.NewCheckButtonWithLabel("Keep current capacity")
		diskGiB := gtk.NewSpinButtonWithRange(24, 1024, 1)
		diskGiB.SetNumeric(true)
		named(diskGiB, "Disk capacity in GiB")
		groupContent.Append(standardDisk)
		diskField := formField("Disk capacity (GiB)", diskGiB)
		standardDisk.ConnectToggled(func() { diskField.SetVisible(!standardDisk.Active()) })
		formHelp("New VMs start at 24 GiB. A larger capacity grows the disk on the next launch. Existing disks are never shrunk.")
		share := entry("Shared folder")
		share.SetEditable(false)
		named(share, "Shared folder")
		shareEnabled := gtk.NewCheckButtonWithLabel("Share this folder with Omarchy")
		groupContent.Append(shareEnabled)
		clearShare := gtk.NewButtonWithLabel("Stop sharing this folder")
		clearShare.ConnectClicked(func() { share.SetText(""); shareEnabled.SetActive(false) })
		groupContent.Append(clearShare)
		chooseShare := gtk.NewButtonWithLabel("Choose a shared folder...")
		groupContent.Append(chooseShare)
		formHelp("Omarchy can read, change and delete files in the folder you share. Access changes apply on the next launch.")
		beginGroup("Display and keyboard", "Advanced")
		scaleChoices := []audioDevice{{"keep", "Keep guest choice"}, {"1", "100%"}, {"1.25", "125%"}, {"1.5", "150%"}, {"2", "200%"}, {"3", "300%"}, {"4", "400%"}}
		scale := gtk.NewDropDownFromStrings([]string{"Follow host display"})
		named(scale, "Guest display scale")
		formField("Guest display scale", scale)
		scaleNames := []string{"auto"}
		keyboardChoices := []audioDevice{{"keep", "Keep guest choice"}, {"us", "English (US)"}, {"us:intl", "English (US, international)"}, {"de", "German"}, {"fr", "French"}, {"es", "Spanish"}}
		keyboard := gtk.NewDropDownFromStrings([]string{"Follow host layout"})
		named(keyboard, "Guest keyboard layout")
		formField("Guest keyboard layout", keyboard)
		keyboardNames := []string{""}
		formHelp("Host layout changes while Omarchy runs apply on its next launch. Press Ctrl+Alt+G to release keyboard capture.")
		beginGroup("Microphone and camera", "Devices")
		microphone := gtk.NewCheckButtonWithLabel("Allow microphone access")
		groupContent.Append(microphone)
		camera := gtk.NewCheckButtonWithLabel("Allow camera access")
		groupContent.Append(camera)
		cameraChoice := gtk.NewDropDownFromStrings([]string{"Automatic"})
		named(cameraChoice, "Camera")
		formField("Camera", cameraChoice)
		cameraNames := []string{""}
		formHelp("Your desktop asks for camera permission when an app inside Omarchy first opens the camera. Capture stops when that app closes it. Access changes apply after shutting down Omarchy and launching it again.")
		beginGroup("Audio", "Devices")
		audioOutput := gtk.NewDropDownFromStrings([]string{"System default"})
		named(audioOutput, "Audio output")
		formField("Audio output", audioOutput)
		audioInput := gtk.NewDropDownFromStrings([]string{"System default"})
		named(audioInput, "Audio input")
		formField("Audio input", audioInput)
		audioOutputNames, audioInputNames := []string{""}, []string{""}
		audioRefresh := gtk.NewButtonWithLabel("Refresh devices")
		groupContent.Append(audioRefresh)
		audioHelp := formHelp("Audio device choices apply when the VM next starts. System default follows your desktop's current device.")
		clipboardGroup := beginGroup("Clipboard", "Devices")
		clipboardShare := gtk.NewCheckButtonWithLabel("Share the clipboard with Omarchy")
		groupContent.Append(clipboardShare)
		clipboardHelp := formHelp("GNOME asks for your permission the first time. A change applies the next time Omarchy starts.")
		beginGroup("Graphics", "Advanced")
		formField("Rendering", render)
		groupContent.Append(defaults)
		formHelp("Automatic tries graphics acceleration and falls back if needed. Changes take effect when the VM next starts.")
		beginGroup("Network and SSH", "Advanced")
		sshEnabled := gtk.NewCheckButtonWithLabel("Allow SSH from this computer")
		groupContent.Append(sshEnabled)
		sshPort := gtk.NewSpinButtonWithRange(1024, 65535, 1)
		sshPort.SetNumeric(true)
		named(sshPort, "SSH port on this computer")
		formField("SSH port", sshPort)
		sshEnabled.ConnectToggled(func() { sshPort.SetSensitive(sshEnabled.Active()) })
		formHelp("SSH starts on the next launch. Connect to 127.0.0.1 on this port with your Omarchy account. Other computers cannot connect.")
		sshKey := entry("SSH public key (optional)")
		sshKey.SetEditable(false)
		named(sshKey, "SSH public key file")
		chooseSSHKey := gtk.NewButtonWithLabel("Choose a public key...")
		chooseSSHKey.ConnectClicked(func() {
			id := current.Request
			dialog := gtk.NewFileDialog()
			dialog.SetTitle("Choose an SSH public key")
			dialog.Open(context.Background(), &window.Window, func(result gio.AsyncResulter) {
				file, err := dialog.OpenFinish(result)
				if answered || current.Prompt != "settings" || id != current.Request {
					return
				}
				if err != nil {
					detail, known := err.(interface {
						Quark() uint32
						ErrorCode() int
					})
					if !known || detail.Quark() != uint32(gtk.DialogErrorQuark()) || detail.ErrorCode() != int(gtk.DialogErrorDismissed) {
						showSettingsProblem("Could not choose that public key: " + err.Error())
					}
					return
				}
				if file.Path() == "" {
					showSettingsProblem("Choose a public key file on a local drive.")
					return
				}
				sshKey.SetText(file.Path())
			})
		})
		groupContent.Append(chooseSSHKey)
		clearSSHKey := gtk.NewButtonWithLabel("Remove public key choice")
		clearSSHKey.ConnectClicked(func() { sshKey.SetText("") })
		groupContent.Append(clearSSHKey)
		formLabel("Other local port forwards (one per line, for example tcp:8080:80)")
		forwards := gtk.NewTextView()
		forwards.SetWrapMode(gtk.WrapWordChar)
		forwards.SetSizeRequest(-1, 88)
		named(forwards, "Other local port forwards, one per line")
		groupContent.Append(forwards)
		settingsContent.Append(form)
		settingsClamp := adw.NewClamp()
		settingsClamp.SetMaximumSize(640)
		settingsClamp.SetTighteningThreshold(640)
		settingsClamp.SetChild(settingsContent)
		settingsScroll := gtk.NewScrolledWindow()
		settingsScroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
		settingsScroll.SetVExpand(true)
		settingsScroll.SetChild(settingsClamp)
		settingsScroll.SetVisible(false)
		form.NotifyProperty("visible-child-name", func() {
			settingsScroll.VAdjustment().SetValue(0)
			settingsScroll.HAdjustment().SetValue(0)
		})
		button := gtk.NewButtonWithLabel("Cancel")
		button.SetHAlign(gtk.AlignStart)
		primary := gtk.NewButton()
		primary.AddCSSClass("suggested-action")
		secondary := gtk.NewButton()
		tertiary := gtk.NewButtonWithLabel("Create diagnostics")
		moveButton := gtk.NewButtonWithLabel("Move this VM...")
		resetButton := gtk.NewButtonWithLabel("Reset this VM...")
		resetButton.AddCSSClass("destructive-action")
		cleanMoveButton := gtk.NewButtonWithLabel("Remove previous copy...")
		cleanResetButton := gtk.NewButtonWithLabel("Remove disk kept from reset...")
		cleanLeftoversButton := gtk.NewButtonWithLabel("Remove unfinished files...")
		primary.SetHExpand(true)
		secondary.SetHExpand(true)
		choices := gtk.NewBox(gtk.OrientationVertical, 8)
		choices.SetHExpand(true)
		for _, action := range []*gtk.Button{primary, secondary, tertiary, moveButton, resetButton, cleanMoveButton, cleanResetButton, cleanLeftoversButton} {
			choices.Append(action)
		}
		choices.SetHAlign(gtk.AlignEnd)
		choices.SetVisible(false)
		// The home and error pages get exactly the buttons the launcher asked for.
		dynamicActions := gtk.NewBox(gtk.OrientationVertical, 8)
		dynamicActions.SetVisible(false)
		actions := gtk.NewBox(gtk.OrientationVertical, 8)
		actions.AddCSSClass("action-footer")
		actions.SetMarginTop(12)
		actions.SetMarginStart(24)
		actions.SetMarginEnd(24)
		actions.SetMarginBottom(16)
		actions.Append(dynamicActions)
		settingsTiming := gtk.NewLabel("")
		settingsTiming.SetWrap(true)
		settingsTiming.SetXAlign(0)
		settingsTiming.AddCSSClass("dim-label")
		settingsTiming.AddCSSClass("setting-help")
		settingsTiming.SetVisible(false)
		actions.Append(settingsTiming)
		footerButtons := newResponsiveRow(button, choices)
		footerButtons.AddCSSClass("footer-buttons")
		actions.Append(footerButtons)
		clamp := adw.NewClamp()
		clamp.SetMaximumSize(640)
		clamp.SetTighteningThreshold(640)
		clamp.SetChild(content)
		page.Append(clamp)
		scroll := gtk.NewScrolledWindow()
		scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
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
		actionClamp.SetMaximumSize(640)
		actionClamp.SetTighteningThreshold(640)
		actionClamp.SetChild(actions)
		layout.Append(actionClamp)
		window.SetContent(layout)
		// Recovery has more actions than fit under its storage summary in a
		// small window, so there they scroll with the page instead of staying
		// pinned above Back.
		choicesInPage := false
		// Move the action group with its wrapper; hidden footer actions must not
		// reserve columns after a different prompt uses them.
		choicesCell := gtk.BaseWidget(choices.Parent()).Object.Cast().(*gtk.FlowBoxChild)
		placeChoices := func(inPage bool) {
			if inPage == choicesInPage {
				return
			}
			if inPage {
				footerButtons.Remove(choicesCell)
				choices.SetHAlign(gtk.AlignFill)
				content.Append(choicesCell)
			} else {
				content.Remove(choicesCell)
				choices.SetHAlign(gtk.AlignEnd)
				footerButtons.Append(choicesCell)
			}
			choicesInPage = inPage
		}
		failed, cancelling, determinate := false, false, false
		var submittedFocus gtk.Widgetter
		reply := func(value string) {
			if current.Request == 0 || answered {
				return
			}
			submittedFocus = window.Window.Focus()
			answered = true
			window.SetDefaultWidget(nil)
			button.SetSensitive(false)
			settingsScroll.SetSensitive(false)
			settingsTabs.SetSensitive(false)
			accountOptions.SetSensitive(false)
			choices.SetSensitive(false)
			dynamicActions.SetSensitive(false)
			menuButton.SetSensitive(false)
			if err := json.NewEncoder(output).Encode(map[string]any{"event": "reply", "request": current.Request, "value": value}); err != nil {
				app.Quit()
			}
		}
		emit := func(event string) {
			if err := json.NewEncoder(output).Encode(map[string]string{"event": event}); err != nil {
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
			data, _ := json.Marshal(settingsForm{ResourceProfile: []string{"balanced", "maximum-performance", "manual"}[min(int(resourceProfile.Selected()), 2)], RefreshAudio: refresh, Memory: memoryValue, CPUs: cpuValue, Render: modes[min(int(render.Selected()), 2)], Fullscreen: fullscreen.Active(), Microphone: microphone.Active(), Camera: camera.Active(), CameraID: cameraNames[min(int(cameraChoice.Selected()), len(cameraNames)-1)], AudioOutput: audioOutputNames[min(int(audioOutput.Selected()), len(audioOutputNames)-1)], AudioInput: audioInputNames[min(int(audioInput.Selected()), len(audioInputNames)-1)], DiskGiB: diskValue, Scale: scaleNames[min(int(scale.Selected()), len(scaleNames)-1)], Keyboard: keyboardNames[min(int(keyboard.Selected()), len(keyboardNames)-1)], SSHEnabled: sshEnabled.Active(), SSHPort: strconv.Itoa(sshPort.ValueAsInt()), SSHKey: sshKey.Text(), Forwards: forwardText, StartAutomatically: startAutomatically.Active(), Share: share.Text(), ShareEnabled: shareEnabled.Active(), CPUMax: current.Settings.CPUMax, ClipboardShare: clipboardShare.Active(), ClipboardAvailable: current.Settings.ClipboardAvailable})
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
				if quickAccount.Active() {
					reply("instant")
				} else {
					reply("personal")
				}
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
		chooseFolder := func() {
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
							if current.Prompt == "settings" {
								showSettingsProblem("Could not choose that folder: " + err.Error())
							} else {
								label.SetText("Could not choose that folder: " + err.Error())
							}
							fmt.Fprintln(os.Stderr, "Folder selection:", err)
						}
						return
					}
					if folder.Path() == "" {
						if current.Prompt == "settings" {
							showSettingsProblem("Choose a folder on a local drive.")
						} else {
							label.SetText("Choose a folder on a local drive.")
						}
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
		}
		secondary.ConnectClicked(chooseFolder)
		chooseShare.ConnectClicked(chooseFolder)
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
			scanner := bufio.NewScanner(input)
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
					previousFocus := submittedFocus
					if previousFocus == nil {
						previousFocus = window.Window.Focus()
					}
					submittedFocus = nil
					pageChanged := current.Prompt != next.Prompt
					preserveManual := current.Prompt == "settings" && next.Prompt == "settings"
					current = next
					answered = false
					settingsScroll.SetSensitive(true)
					settingsTabs.SetSensitive(true)
					accountOptions.SetSensitive(true)
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
					pageTitle.SetText("")
					pageIcon.SetVisible(next.Error || next.Prompt == "error")
					scroll.SetVisible(next.Prompt != "home" && next.Prompt != "settings")
					homeScroll.SetVisible(next.Prompt == "home")
					settingsScroll.SetVisible(next.Prompt == "settings")
					productVersion.SetText("LINUX" + platformVersion(next.Version))
					settingsTabs.SetVisible(next.Prompt == "settings")
					settingsTiming.SetVisible(next.Prompt == "settings")
					settingsStatus.SetVisible(next.Status != "")
					accountOptions.SetVisible(next.Prompt == "account")
					if pageChanged {
						form.SetVisibleChildName("General")
						personalAccount.SetActive(true)
					}
					homeHeadline.SetText(next.Headline)
					homeHeadline.SetVisible(next.Headline != "")
					homeStatus.SetText(next.Status)
					homeDetail.SetText(next.Detail)
					homeDetail.SetVisible(next.Detail != "")
					settingsStatus.SetText(next.Status)
					secondary.RemoveCSSClass("destructive-action")
					visibleSections, setupSections := homeSectionsForState(next)
					fillSections(homeSections, visibleSections)
					fillSections(homeSetup, setupSections)
					homeDetails.SetVisible(len(setupSections) > 0)
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
						pageTitle.SetText("Backup and recovery")
						primary.SetLabel("Create backup")
						secondary.SetLabel("Restore as a copy")
						button.SetLabel("Back")
					case "forget-location":
						pageTitle.SetText("Forget saved location?")
						primary.SetLabel("Keep saved location")
						secondary.SetLabel("Forget saved location")
						secondary.AddCSSClass("destructive-action")
						button.SetLabel("Cancel")
					case "delete-default":
						pageTitle.SetText("Delete this VM?")
						primary.SetLabel("Keep this VM")
						secondary.SetLabel("Delete VM and system files")
						secondary.AddCSSClass("destructive-action")
						button.SetLabel("Cancel")
					case "backup-folder", "restore-parent", "restore-archive", "attach-folder", "move-folder":
						pageTitle.SetText("Backup and recovery")
						if next.Prompt == "attach-folder" {
							pageTitle.SetText("Use an existing VM")
						}
						if next.Prompt == "move-folder" {
							pageTitle.SetText("Move this VM")
						}
						primary.SetLabel("Choose location...")
						button.SetLabel("Cancel")
					case "choice":
						pageTitle.SetText(next.Title)
						primary.SetLabel(next.Primary)
						secondary.SetLabel(next.Secondary)
						if next.Destructive {
							secondary.AddCSSClass("destructive-action")
						}
						button.SetLabel("Cancel")
					case "about":
						pageTitle.SetText("About Try Omarchy")
						primary.SetLabel("Back")
						button.SetLabel("Back")
						if next.Status == "" {
							next.Status = "Try Omarchy for Linux, version " + next.Version + "."
						}
					case "settings-saved":
						pageTitle.SetText("Settings saved")
						primary.SetLabel("Done")
						button.SetLabel("Close")
					case "settings":
						pageTitle.SetText("Settings")
						primary.SetLabel("Save settings")
						secondary.SetLabel("Choose a shared folder...")
						if next.Settings != nil {
							settingsTiming.SetText(settingsFooterText(next.Settings.Running, next.Settings.AudioLive))
							memoryMiB, _ := strconv.Atoi(next.Settings.Memory)
							cpuCount, _ := strconv.Atoi(next.Settings.CPUs)
							autoMemory.SetActive(memoryMiB == 0)
							autoCPUs.SetActive(cpuCount == 0)
							if !preserveManual || memoryMiB != 0 {
								memory.SetValue(float64(max(memoryMiB, 1024)) / 1024)
							}
							memory.SetVisible(memoryMiB != 0)
							switch next.Settings.ResourceProfile {
							case "maximum-performance":
								resourceProfile.SetSelected(1)
							case "manual":
								resourceProfile.SetSelected(2)
							default:
								if next.Settings.ResourceProfile == "" && (memoryMiB != 0 || cpuCount != 0) {
									resourceProfile.SetSelected(2)
								} else {
									resourceProfile.SetSelected(0)
								}
							}
							automaticSummary = next.Settings.ResourceSummary
							updateResources()
							if resourceProfile.Selected() == 0 {
								resourceSummary.SetText(automaticSummary)
							}
							cpus.SetRange(1, float64(max(next.Settings.CPUMax, 1)))
							if !preserveManual || cpuCount != 0 {
								cpus.SetValue(float64(max(cpuCount, 1)))
							}
							cpus.SetVisible(cpuCount != 0)
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
							if !preserveManual || diskCapacity != 0 {
								diskGiB.SetValue(float64(max(diskCapacity, 24)))
							}
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
							clipboardGroup.SetVisible(next.Settings.ClipboardAvailable)
							clipboardShare.SetVisible(next.Settings.ClipboardAvailable)
							clipboardHelp.SetVisible(next.Settings.ClipboardAvailable)
							share.SetText(next.Settings.Share)
							shareEnabled.SetActive(next.Settings.ShareEnabled)
						}
					case "location":
						pageTitle.SetText("Where should Omarchy live?")
						primary.SetLabel("Use default location")
						secondary.SetLabel("Choose another folder...")
					case "account":
						pageTitle.SetText("Choose your first launch")
						primary.SetLabel("Continue")
					case "share":
						pageTitle.SetText("Share a folder with Omarchy?")
						primary.SetLabel("Not now")
						secondary.SetLabel("Choose a shared folder...")
						if next.Status == "" {
							next.Status = "Omarchy can read, change, and delete files in the folder you choose. Use a dedicated folder to exchange files."
						}
					case "grant-files":
						pageTitle.SetText("Share dropped files")
						primary.SetLabel("Choose files...")
						button.SetLabel("Cancel")
					case "close":
						pageTitle.SetText("Shut down Omarchy?")
						primary.SetLabel("Keep running")
						secondary.SetLabel("Shut down")
						secondary.AddCSSClass("destructive-action")
						if next.Status == "" {
							next.Status = "Save your work inside Omarchy before shutting down."
						}
					case "error":
						pageTitle.SetText(orDefault(next.ErrorTitle, "Omarchy could not start"))
					}
					secondary.SetVisible(next.Prompt != "account" && next.Prompt != "settings" && !oneButtonPrompt(next.Prompt) && next.Prompt != "error")
					choicesCell.SetVisible(choices.Visible())
					gtk.BaseWidget(button.Parent()).SetVisible(button.Visible())
					footerButtons.SetVisible(!dynamic)
					// The home's buttons, or an error page's, in the order asked for.
					dynamicList := next.Actions
					if next.Prompt == "error" {
						dynamicList = errorActions(next)
					}
					var focus *gtk.Button
					clearChildren(dynamicActions)
					secondaryActions := gtk.NewFlowBox()
					secondaryActions.SetSelectionMode(gtk.SelectionNone)
					secondaryActions.SetHomogeneous(true)
					secondaryActions.SetMinChildrenPerLine(1)
					secondaryActions.SetMaxChildrenPerLine(3)
					secondaryActions.SetColumnSpacing(8)
					secondaryActions.SetRowSpacing(8)
					for i, item := range dynamicList {
						item := item
						b := gtk.NewButtonWithLabel(item.Label)
						if item.Suggested {
							b.AddCSSClass("suggested-action")
						}
						if item.Destructive {
							b.AddCSSClass("destructive-action")
						}
						if item.Reply == "close" {
							b.AddCSSClass("flat")
						}
						b.ConnectClicked(func() { reply(item.Reply) })
						if item.Suggested {
							dynamicActions.Append(b)
						} else {
							b.SetHExpand(true)
							child := gtk.NewFlowBoxChild()
							child.SetFocusable(false)
							child.SetChild(b)
							secondaryActions.Append(child)
						}
						if i == homeSuggested(dynamicList) {
							focus = b
						}
					}
					if secondaryActions.FirstChild() != nil {
						dynamicActions.Append(secondaryActions)
					}
					window.SetDefaultWidget(nil)
					if preserveManual && previousFocus != nil {
						window.SetDefaultWidget(primary)
						gtk.BaseWidget(previousFocus).GrabFocus()
					} else if focus != nil {
						window.SetDefaultWidget(focus)
						focus.GrabFocus()
					} else if next.Prompt == "account" {
						window.SetDefaultWidget(primary)
						personalAccount.GrabFocus()
					} else if next.Prompt == "recovery" {
						button.GrabFocus()
					} else if next.Prompt != "" {
						window.SetDefaultWidget(primary)
						primary.GrabFocus()
					}
					failed = next.Error
					if next.Prompt == "" {
						label.AddCSSClass("title-3")
					} else {
						label.RemoveCSSClass("title-3")
					}
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
						pageTitle.SetText(orDefault(next.ErrorTitle, "Omarchy could not start"))
						button.SetLabel("Close")
					}
					pageTitle.SetVisible(pageTitle.Text() != "")
					label.SetVisible(next.Status != "")
					if pageChanged {
						for _, pane := range []*gtk.ScrolledWindow{scroll, homeScroll, settingsScroll} {
							pane.VAdjustment().SetValue(0)
							pane.HAdjustment().SetValue(0)
						}
					}
					wrapWindowLabels(&window.Window.Widget)
					page.QueueResize()
				})
			}
			// EOF also closes the window when the launcher exits or crashes.
			glib.IdleAdd(app.Quit)
		}()
		wrapWindowLabels(&window.Window.Widget)
		window.Present()
		emit("ready")
		if onWindow != nil {
			onWindow(window)
		}
	})
	return app.Run([]string{os.Args[0]})
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
