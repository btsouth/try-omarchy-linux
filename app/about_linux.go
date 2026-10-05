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
		Status: uiTextWith("about.linux.try_omarchy_for_linux_runs_omarchy_in_a", map[string]string{"value": linuxVersionLabel(linuxAppVersion)}),
		Sections: []linuxSection{
			{Heading: uiText("about.linux.keyboard_mouse_and_window"), Rows: []linuxRow{
				{Title: uiText("about.linux.get_your_keyboard_back"), Detail: uiText("about.linux.while_the_omarchy_window_is_focused_it_takes")},
				{Title: uiText("about.linux.if_super_opens_your_desktop_s_overview"), Detail: uiText("about.linux.your_desktop_asks_once_whether_try_omarchy_may")},
				{Title: uiText("about.linux.fullscreen"), Detail: uiText("about.linux.press_ctrl_alt_f_to_make_the_omarchy")},
				{Title: uiText("about.linux.omarchy_s_own_menu"), Detail: uiText("about.linux.inside_omarchy_super_space_opens_its_menu_and")},
			}},
			{Heading: uiText("about.linux.files"), Rows: []linuxRow{
				{Title: uiText("about.linux.copy_paste_and_drop"), Detail: uiText("about.linux.text_images_and_files_move_both_ways_through")},
				{Title: uiText("about.linux.a_shared_folder"), Detail: uiText("about.linux.settings_can_share_one_folder_with_omarchy_where")},
				{Title: uiText("settings.linux.disk_space"), Detail: uiText("about.linux.deleting_files_inside_omarchy_does_not_shrink_its")},
			}},
			{Heading: uiText("about.linux.settings_and_devices"), Rows: []linuxRow{
				{Title: uiText("settings.section.resources"), Detail: uiText("about.linux.balanced_leaves_room_for_your_linux_desktop_maximum")},
				{Title: uiText("about.linux.when_changes_apply"), Detail: uiText("about.linux.audio_devices_and_local_port_forwards_can_change")},
				{Title: uiText("about.linux.if_save_fails"), Detail: uiText("about.linux.settings_identifies_any_groups_already_saved_and_keeps")},
			}},
			{Heading: uiText("about.linux.removing_the_app_and_your_vm"), Rows: []linuxRow{
				{Title: uiText("about.linux.uninstalling_the_app"), Detail: uiText("about.linux.your_vm_stays_if_you_keep_the_app")},
				{Title: uiText("about.linux.deleting_a_vm"), Detail: uiText("about.linux.delete_this_vm_in_the_menu_on_the")},
				{Title: uiText("launcher.linux.snapshots"), Detail: uiText("about.linux.backup_and_recovery_snapshots_saves_the_vm_inside")},
				{Title: uiText("about.linux.coming_back"), Detail: uiText("about.linux.after_reinstalling_your_vm_opens_as_before_if")},
			}},
			{Heading: uiText("about.linux.updates"), Rows: []linuxRow{
				{Title: uiText("about.linux.the_app"), Detail: uiText("about.linux.try_omarchy_updates_through_software_or_flatpak_update")},
				{Title: uiText("about.linux.omarchy_s_system_files"), Detail: uiText("about.linux.a_new_app_version_can_bring_newer_system")},
			}},
			{Heading: uiText("about.linux.this_version"), Rows: []linuxRow{
				{Title: uiText("brand.name"), Detail: linuxVersionLabel(linuxAppVersion)},
				{Title: uiText("about.linux.omarchy_system_files"), Detail: releaseVersion(linuxGuestReleaseURL)},
				{Title: uiText("about.linux.help_and_reports"), Detail: strings.TrimPrefix(linuxHelpPage[:strings.Index(linuxHelpPage, "/blob/")], "https://")},
			}},
		}}
}
