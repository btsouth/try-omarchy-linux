//go:build linux

package main

import "strings"

// linuxVersionLabel writes a version tag the way a person reads it:
// v0.1.0-preview.2 becomes 0.1.0 preview 2.
func linuxVersionLabel(version string) string {
	version = strings.TrimPrefix(version, "v")
	return strings.NewReplacer("-preview.", " preview ", "-", " ").Replace(version)
}

// linuxAboutState is the help page. It answers the questions people have in
// the first hour: how to get the keyboard back, how files move, what removing
// the app does to their VM, and how updates arrive.
func linuxAboutState() linuxSetupState {
	return linuxSetupState{Prompt: "about", Version: linuxAppVersion,
		Status: "Try Omarchy for Linux " + linuxVersionLabel(linuxAppVersion) + " runs Omarchy in a virtual machine on this computer.",
		Sections: []linuxSection{
			{Heading: "Keyboard, mouse and window", Rows: []linuxRow{
				{Title: "Get your keyboard back", Detail: "While the Omarchy window is focused it takes your keyboard, including the Super key. Press Ctrl+Alt+G, or click another window, to use your desktop's shortcuts again. Click the Omarchy window to give it the keyboard back."},
				{Title: "If Super opens your desktop's overview", Detail: "Your desktop asks once whether Try Omarchy may use its shortcuts. If that was refused, allow it again in your desktop's settings; on GNOME, turn on Inhibit Shortcuts in Settings, Apps, Try Omarchy."},
				{Title: "Fullscreen", Detail: "Press Ctrl+Alt+F to make the Omarchy window fullscreen, and again to leave. Settings can open it fullscreen every time."},
				{Title: "Omarchy's own menu", Detail: "Inside Omarchy, Super+Space opens its menu and Super+K shows every key binding."},
			}},
			{Heading: "Files", Rows: []linuxRow{
				{Title: "Copy, paste and drop", Detail: "Text, images and files move both ways through the clipboard, and dropping files on the window sends them to Omarchy. On GNOME, clipboard sharing asks your permission once."},
				{Title: "A shared folder", Detail: "Settings can share one folder with Omarchy, where it appears as /mnt/host. Omarchy can change what is in it. Nothing else on your computer is visible to Omarchy."},
			}},
			{Heading: "Settings and devices", Rows: []linuxRow{
				{Title: "Resources", Detail: "Balanced leaves room for your Linux desktop. Maximum performance uses more available resources. Manual lets you tune memory and processors. Changes apply on the next VM launch."},
				{Title: "When changes apply", Detail: "Audio devices can switch when you save during a running session. Camera and microphone access, shared folders, disk, display, keyboard and network settings need a shutdown and launch. A guest reboot does not apply them. Startup behavior applies the next time you open Try Omarchy."},
				{Title: "If Save fails", Detail: "Settings identifies any groups already saved and keeps your remaining edits. Fix the folder's permissions or free space, then Save again. If a live audio switch fails, the saved choices remain available for the next launch."},
			}},
			{Heading: "Removing the app and your VM", Rows: []linuxRow{
				{Title: "Uninstalling the app", Detail: "Your VM stays if you keep the app's data: choose Keep under App Settings & Data in Software, or use flatpak uninstall without --delete-data. Choosing Delete also removes a VM stored in the app's own storage. A VM in a folder you chose is never touched."},
				{Title: "Deleting a VM", Detail: "Delete this VM, in the menu on the home screen, removes only the VM in the app's own storage. To delete a folder you chose, remove it in your file manager. Backups are ordinary .zip files; delete them yourself when you no longer need them."},
				{Title: "Coming back", Detail: "After reinstalling, your VM opens as before. If you deleted the app's data, choose Use existing data folder and pick the folder that holds your VM."},
			}},
			{Heading: "Updates", Rows: []linuxRow{
				{Title: "The app", Detail: "Try Omarchy updates through Software or flatpak update, like other Flatpak apps."},
				{Title: "Omarchy's system files", Detail: "A new app version can bring newer system files. Try Omarchy downloads them at the next launch, keeps your files, and goes back to the old ones if the new ones do not start. Updates inside Omarchy (Super+Space, then Update) are separate."},
			}},
			{Heading: "This version", Rows: []linuxRow{
				{Title: "Try Omarchy", Detail: linuxVersionLabel(linuxAppVersion)},
				{Title: "Omarchy system files", Detail: releaseVersion(linuxGuestReleaseURL)},
				{Title: "Help and reports", Detail: strings.TrimPrefix(linuxHelpPage[:strings.Index(linuxHelpPage, "/blob/")], "https://")},
			}},
		}}
}
