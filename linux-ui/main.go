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
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
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
	Title         string `json:"title"`
	Primary       string `json:"primary"`
	Secondary     string `json:"secondary"`
	Destructive   bool   `json:"destructive"`
	CanMove       bool   `json:"canMove"`
	CanReset      bool   `json:"canReset"`
	CanCleanMove  bool   `json:"canCleanMove"`
	CanCleanReset bool   `json:"canCleanReset"`
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
	AudioOutput        string        `json:"audioOutput"`
	AudioInput         string        `json:"audioInput"`
	AudioOutputs       []audioDevice `json:"audioOutputs"`
	AudioInputs        []audioDevice `json:"audioInputs"`
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

func displayPath(path string) string {
	const width = 42
	runes := []rune(path)
	var lines []string
	for len(runes) > width {
		split := width
		for i := width - 1; i >= width/2; i-- {
			if runes[i] == '/' {
				split = i + 1
				break
			}
		}
		lines = append(lines, string(runes[:split]))
		runes = runes[split:]
	}
	lines = append(lines, string(runes))
	return strings.Join(lines, "\n")
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
		layout.Append(header)
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
		progress := gtk.NewProgressBar()
		content.Append(progress)
		homeContent := gtk.NewBox(gtk.OrientationVertical, 12)
		homeContent.SetMarginTop(16)
		homeContent.SetMarginBottom(24)
		homeContent.SetMarginStart(24)
		homeContent.SetMarginEnd(24)
		homeIcon := gtk.NewImageFromIconName("com.tryomarchy.TryOmarchy")
		homeIcon.SetPixelSize(64)
		homeIcon.SetHAlign(gtk.AlignCenter)
		homeContent.Append(homeIcon)
		homeTitle := gtk.NewLabel("Try Omarchy")
		homeTitle.AddCSSClass("title-1")
		homeContent.Append(homeTitle)
		homeStatus := gtk.NewLabel("")
		homeStatus.SetWrap(true)
		homeStatus.SetMaxWidthChars(48)
		homeStatus.SetJustify(gtk.JustifyCenter)
		homeContent.Append(homeStatus)
		homePath := gtk.NewLabel("")
		homePath.SetWrap(true)
		homePath.SetWrapMode(pango.WrapWordChar)
		homePath.SetSelectable(true)
		homePath.SetMaxWidthChars(48)
		homePath.SetJustify(gtk.JustifyCenter)
		homeContent.Append(homePath)
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
		form := gtk.NewBox(gtk.OrientationVertical, 10)
		entry := func(title string) *gtk.Entry {
			titleLabel := gtk.NewLabel(title)
			titleLabel.SetHAlign(gtk.AlignStart)
			form.Append(titleLabel)
			input := gtk.NewEntry()
			form.Append(input)
			return input
		}
		formLabel := func(title string) {
			l := gtk.NewLabel(title)
			l.SetHAlign(gtk.AlignStart)
			form.Append(l)
		}
		formLabel("Memory for Omarchy (GiB)")
		autoMemory := gtk.NewCheckButtonWithLabel("Choose memory automatically")
		memory := gtk.NewSpinButtonWithRange(1, 64, 0.25)
		memory.SetDigits(2)
		memory.SetNumeric(true)
		form.Append(autoMemory)
		form.Append(memory)
		autoMemory.ConnectToggled(func() { memory.SetSensitive(!autoMemory.Active()) })
		formLabel("Processors")
		autoCPUs := gtk.NewCheckButtonWithLabel("Choose processors automatically")
		cpus := gtk.NewSpinButtonWithRange(1, 64, 1)
		cpus.SetNumeric(true)
		form.Append(autoCPUs)
		form.Append(cpus)
		autoCPUs.ConnectToggled(func() { cpus.SetSensitive(!autoCPUs.Active()) })
		formLabel("Rendering")
		render := gtk.NewDropDownFromStrings([]string{"Automatic (recommended)", "Graphics acceleration", "Software rendering"})
		form.Append(render)
		description := gtk.NewLabel("Automatic tries graphics acceleration and falls back if needed. Changes take effect when the VM next starts.")
		description.SetWrap(true)
		description.SetHAlign(gtk.AlignStart)
		form.Append(description)
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
		form.Append(standardDisk)
		form.Append(diskGiB)
		standardDisk.ConnectToggled(func() { diskGiB.SetSensitive(!standardDisk.Active()) })
		diskHelp := gtk.NewLabel("A larger capacity grows the disk on the next launch. Existing disks are never shrunk.")
		diskHelp.SetWrap(true)
		diskHelp.SetHAlign(gtk.AlignStart)
		form.Append(diskHelp)
		formLabel("Display and audio")
		fullscreen := gtk.NewCheckButtonWithLabel("Open Omarchy fullscreen")
		form.Append(fullscreen)
		formLabel("Guest display scale")
		scaleChoices := []audioDevice{{"keep", "Keep guest choice"}, {"1", "100%"}, {"1.25", "125%"}, {"1.5", "150%"}, {"2", "200%"}, {"3", "300%"}, {"4", "400%"}}
		scale := gtk.NewDropDownFromStrings([]string{"Follow host display"})
		form.Append(scale)
		scaleNames := []string{"auto"}
		formLabel("Guest keyboard layout")
		keyboardChoices := []audioDevice{{"keep", "Keep guest choice"}, {"us", "English (US)"}, {"us:intl", "English (US, international)"}, {"de", "German"}, {"fr", "French"}, {"es", "Spanish"}}
		keyboard := gtk.NewDropDownFromStrings([]string{"Follow host layout"})
		form.Append(keyboard)
		keyboardNames := []string{""}
		keyboardHelp := gtk.NewLabel("Host layout changes while Omarchy runs apply on its next launch. Press Ctrl+Alt+G to release keyboard capture.")
		keyboardHelp.SetWrap(true)
		keyboardHelp.SetHAlign(gtk.AlignStart)
		form.Append(keyboardHelp)
		microphone := gtk.NewCheckButtonWithLabel("Allow microphone access")
		form.Append(microphone)
		formLabel("Audio output")
		audioOutput := gtk.NewDropDownFromStrings([]string{"System default"})
		form.Append(audioOutput)
		formLabel("Audio input")
		audioInput := gtk.NewDropDownFromStrings([]string{"System default"})
		form.Append(audioInput)
		audioOutputNames, audioInputNames := []string{""}, []string{""}
		audioHelp := gtk.NewLabel("Audio device choices apply when the VM next starts. System default follows your desktop's current device.")
		audioHelp.SetWrap(true)
		audioHelp.SetHAlign(gtk.AlignStart)
		form.Append(audioHelp)
		formLabel("Network and SSH")
		sshEnabled := gtk.NewCheckButtonWithLabel("Allow SSH from this computer")
		form.Append(sshEnabled)
		sshPort := gtk.NewSpinButtonWithRange(1024, 65535, 1)
		sshPort.SetNumeric(true)
		form.Append(sshPort)
		sshEnabled.ConnectToggled(func() { sshPort.SetSensitive(sshEnabled.Active()) })
		sshHelp := gtk.NewLabel("SSH starts on the next launch. Connect to 127.0.0.1 on this port with your Omarchy account. Other computers cannot connect.")
		sshHelp.SetWrap(true)
		sshHelp.SetHAlign(gtk.AlignStart)
		form.Append(sshHelp)
		sshKey := entry("SSH public key (optional)")
		sshKey.SetEditable(false)
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
		form.Append(forwards)
		formLabel("Startup")
		startAutomatically := gtk.NewCheckButtonWithLabel("Start Omarchy when I open Try Omarchy")
		form.Append(startAutomatically)
		startupHelp := gtk.NewLabel("The launcher stays open for 10 seconds so you can choose Settings or Close.")
		startupHelp.SetWrap(true)
		startupHelp.SetHAlign(gtk.AlignStart)
		form.Append(startupHelp)
		share := entry("Shared folder")
		share.SetEditable(false)
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
		tertiary := gtk.NewButtonWithLabel("About and help")
		quaternary := gtk.NewButtonWithLabel("Backup and recovery")
		quinary := gtk.NewButtonWithLabel("Use existing data folder")
		forgetButton := gtk.NewButtonWithLabel("Forget unavailable location")
		deleteButton := gtk.NewButtonWithLabel("Delete this VM...")
		moveButton := gtk.NewButtonWithLabel("Move this VM...")
		resetButton := gtk.NewButtonWithLabel("Reset this VM...")
		cleanMoveButton := gtk.NewButtonWithLabel("Remove previous copy...")
		cleanResetButton := gtk.NewButtonWithLabel("Remove disk kept from reset...")
		choices.Append(primary)
		choices.Append(secondary)
		choices.Append(tertiary)
		choices.Append(quaternary)
		choices.Append(quinary)
		choices.Append(forgetButton)
		choices.Append(deleteButton)
		choices.Append(moveButton)
		choices.Append(resetButton)
		choices.Append(cleanMoveButton)
		choices.Append(cleanResetButton)
		choices.SetVisible(false)
		actions := gtk.NewBox(gtk.OrientationVertical, 8)
		actions.SetMarginStart(24)
		actions.SetMarginEnd(24)
		actions.SetMarginBottom(16)
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
		actionClamp := adw.NewClamp()
		actionClamp.SetMaximumSize(480)
		actionClamp.SetChild(actions)
		layout.Append(actionClamp)
		window.SetContent(layout)
		failed, cancelling, determinate := false, false, false
		var current state
		answered := false
		reply := func(value string) {
			if current.Request == 0 || answered {
				return
			}
			answered = true
			choices.SetSensitive(false)
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
			if current.Prompt == "home" {
				reply("close")
			} else if current.Prompt == "about" || current.Prompt == "settings-saved" {
				reply("back")
			} else if current.Prompt == "close" {
				reply("keep")
			} else if current.Prompt == "forget-location" {
				reply("keep")
			} else if current.Prompt == "delete-default" {
				reply("keep")
			} else if current.Prompt == "recovery" {
				reply("back")
			} else if current.Prompt == "settings" || current.Prompt == "grant-files" || current.Prompt == "backup-folder" || current.Prompt == "restore-archive" || current.Prompt == "restore-parent" || current.Prompt == "attach-folder" || current.Prompt == "move-folder" || current.Prompt == "choice" {
				reply("cancel")
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
		primary.ConnectClicked(func() {
			switch current.Prompt {
			case "home":
				reply("launch")
			case "about":
				reply("back")
			case "settings-saved":
				reply("back")
			case "location":
				reply("default")
			case "account":
				reply("instant")
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
				data, _ := json.Marshal(settingsForm{Memory: memoryValue, CPUs: cpuValue, Render: modes[min(int(render.Selected()), 2)], Fullscreen: fullscreen.Active(), Microphone: microphone.Active(), AudioOutput: audioOutputNames[min(int(audioOutput.Selected()), len(audioOutputNames)-1)], AudioInput: audioInputNames[min(int(audioInput.Selected()), len(audioInputNames)-1)], DiskGiB: diskValue, Scale: scaleNames[min(int(scale.Selected()), len(scaleNames)-1)], Keyboard: keyboardNames[min(int(keyboard.Selected()), len(keyboardNames)-1)], SSHEnabled: sshEnabled.Active(), SSHPort: strconv.Itoa(sshPort.ValueAsInt()), SSHKey: sshKey.Text(), Forwards: forwardText, StartAutomatically: startAutomatically.Active(), Share: share.Text(), ShareEnabled: shareEnabled.Active(), CPUMax: current.Settings.CPUMax})
				reply(string(data))
			}
		})
		secondary.ConnectClicked(func() {
			switch current.Prompt {
			case "home":
				reply("settings")
			case "account":
				reply("personal")
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
			if current.Prompt == "home" {
				reply("about")
			} else if current.Prompt == "recovery" {
				reply("diagnostics")
			}
		})
		quaternary.ConnectClicked(func() {
			if current.Prompt == "home" {
				reply("recovery")
			} else if current.Prompt == "recovery" {
				reply("back")
			}
		})
		quinary.ConnectClicked(func() {
			if current.Prompt == "home" && current.CanAttach {
				reply("attach")
			}
		})
		forgetButton.ConnectClicked(func() {
			if current.Prompt == "home" && current.CanForget {
				reply("forget")
			}
		})
		deleteButton.ConnectClicked(func() {
			if current.Prompt == "home" && current.CanDelete {
				reply("delete-default")
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
					button.SetSensitive(true)
					window.SetDeletable(!next.NonCancellable)
					primary.SetSensitive(next.Prompt != "home" || !next.CanForget)
					secondary.SetSensitive(next.Prompt != "home" || !next.CanForget)
					quaternary.SetSensitive(next.Prompt != "home" || !next.CanForget)
					choices.SetVisible(next.Prompt != "")
					button.SetVisible(!next.NonCancellable && next.Prompt != "close" && next.Prompt != "about" && next.Prompt != "settings-saved")
					tertiary.SetVisible(next.Prompt == "home" || next.Prompt == "recovery")
					quaternary.SetVisible(next.Prompt == "home")
					quinary.SetVisible(next.Prompt == "home" && next.CanAttach)
					forgetButton.SetVisible(next.Prompt == "home" && next.CanForget)
					deleteButton.SetVisible(next.Prompt == "home" && next.CanDelete)
					moveButton.SetVisible(next.Prompt == "recovery" && next.CanMove)
					resetButton.SetVisible(next.Prompt == "recovery" && next.CanReset)
					cleanMoveButton.SetVisible(next.Prompt == "recovery" && next.CanCleanMove)
					cleanResetButton.SetVisible(next.Prompt == "recovery" && next.CanCleanReset)
					tertiary.SetLabel("About and help")
					quaternary.SetLabel("Backup and recovery")
					button.SetLabel("Cancel")
					if next.Booting && next.Prompt == "" {
						button.SetLabel("Stop Omarchy")
					}
					page.SetTitle("Try Omarchy")
					page.SetIconName("com.tryomarchy.TryOmarchy")
					scroll.SetVisible(next.Prompt != "home" && next.Prompt != "settings")
					homeScroll.SetVisible(next.Prompt == "home")
					settingsScroll.SetVisible(next.Prompt == "settings")
					homeStatus.SetText(next.Status)
					homePath.SetText("Storage: " + next.Path)
					settingsStatus.SetText(next.Status)
					secondary.RemoveCSSClass("destructive-action")
					switch next.Prompt {
					case "home":
						page.SetTitle("Try Omarchy")
						primary.SetLabel("Launch Omarchy")
						secondary.SetLabel("Settings")
						quinary.SetLabel("Use existing data folder")
						button.SetLabel("Close")
					case "recovery":
						page.SetTitle("Backup and recovery")
						page.SetIconName("")
						primary.SetLabel("Create backup")
						secondary.SetLabel("Restore as a copy")
						tertiary.SetLabel("Create diagnostics")
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
						secondary.SetLabel("Delete VM and guest files")
						secondary.AddCSSClass("destructive-action")
						button.SetLabel("Cancel")
					case "backup-folder", "restore-parent", "restore-archive", "attach-folder", "move-folder":
						page.SetTitle("Backup and recovery")
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
						secondary.SetVisible(false)
						next.Status = "Try Omarchy for Linux, version " + next.Version + ". It runs Omarchy in a VM stored on this computer. Settings apply at the next start. On GNOME Wayland, clipboard sync asks for pointer and clipboard permission; Try Omarchy never sends pointer events. Uninstall keeps external VM folders. After reinstall, choose Use existing data folder to reopen one. Help: github.com/btsouth/try-omarchy-linux"
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
							audioOutputNames = outputNames
							audioOutput.SetModel(gtk.NewStringList(outputLabels))
							audioOutput.SetSelected(outputIndex)
							inputLabels, inputNames, inputIndex := namedChoices("System default", "Unavailable: ", next.Settings.AudioInputs, next.Settings.AudioInput)
							audioInputNames = inputNames
							audioInput.SetModel(gtk.NewStringList(inputLabels))
							audioInput.SetSelected(inputIndex)
							share.SetText(next.Settings.Share)
							shareEnabled.SetActive(next.Settings.ShareEnabled)
						}
					case "location":
						page.SetTitle("Where should Omarchy live?")
						primary.SetLabel("Use default location")
						secondary.SetLabel("Choose another folder...")
						if next.Status == "" {
							next.Status = "Your virtual machine and settings stay in this folder. A try-omarchy folder is created if you choose another location.\n\nDefault location:\n" + displayPath(next.Path)
						}
					case "account":
						page.SetTitle("Make yourself at home")
						primary.SetLabel("Try it now")
						secondary.SetLabel("Set up my own account")
						next.Status = "Start right away with the trial account (username and password: omarchy), or choose your own name and password inside Omarchy."
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
						next.Status = "Save your work inside Omarchy before shutting down."
					}
					secondary.SetVisible(next.Prompt != "about" && next.Prompt != "settings-saved" && next.Prompt != "grant-files" && next.Prompt != "backup-folder" && next.Prompt != "restore-parent" && next.Prompt != "restore-archive" && next.Prompt != "attach-folder" && next.Prompt != "move-folder")
					if next.Prompt != "" {
						primary.GrabFocus()
					}
					failed = next.Error
					label.SetText(next.Status)
					label.SetSelectable(failed)
					determinate = next.Total > 0
					progress.SetShowText(determinate)
					progress.SetVisible(!failed && next.Prompt == "")
					if determinate {
						fraction := min(max(float64(next.Current)/float64(next.Total), 0), 1)
						progress.SetFraction(fraction)
						progress.SetText(fmt.Sprintf("%.0f%%", fraction*100))
					} else {
						progress.SetText("")
					}
					if failed {
						title := next.ErrorTitle
						if title == "" {
							title = "Omarchy could not start"
						}
						page.SetTitle(title)
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
