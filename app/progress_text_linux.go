//go:build linux

package main

import (
	"path/filepath"
)

// The launcher reports engineering steps ("Starting Omarchy - GPU accelerated
// (virgl + Venus Vulkan)"). The window says what a person is waiting for, and
// only shows a bar when there is real progress to measure. The terminal and
// the log keep the raw text.

type linuxStage int

const (
	stageOther    linuxStage = iota
	stagePrepare             // small files, checks and hand-offs: nothing to measure
	stageDownload            // the system image, measured in bytes
	stageCheck               // verifying what was downloaded or unpacked
	stageUnpack
	stageDisk
	stageStart
	stageDesktop
)

// linuxFriendlyStatus adapts a catalog message by identity, never its wording.
// Small preparation files do not show a bar; the system image does.
func linuxFriendlyStatus(key string, values map[string]string, updating bool) (string, linuxStage) {
	switch key {
	case "status.downloading_omarchy":
		return linuxDownloadStatus(updating, values["part"] == values["total"])
	case "launcher.linux.preparing_omarchy":
		return linuxDownloadStatus(updating, false)
	case "status.preparing_image_update":
		return uiText("progress.linux.getting_the_omarchy_update_ready"), stagePrepare
	case "status.resuming_file":
		if filepath.Base(values["file"]) == "rootfs.ext4.zst" {
			return linuxDownloadStatus(updating, true)
		}
		return linuxDownloadStatus(updating, false)
	case "status.checking_cached_file", "status.checking_downloaded_file":
		if filepath.Base(values["file"]) == "rootfs.ext4.zst" {
			return uiText("progress.linux.checking_the_download"), stageCheck
		}
		return linuxDownloadStatus(updating, false)
	case "status.checking_cached_system":
		return uiText("progress.linux.checking_omarchy_s_system_files"), stageCheck
	case "status.unpacking_system":
		return uiText("progress.linux.unpacking_omarchy"), stageUnpack
	case "status.checking_unpacked_system":
		return uiText("progress.linux.checking_the_unpacked_files"), stageCheck
	case "status.ready_starting":
		return uiText("progress.linux.almost_ready"), stagePrepare
	case "status.preparing_disk":
		return uiText("progress.linux.creating_your_omarchy_disk"), stageDisk
	case "status.linux.starting", "status.linux.booting":
		return uiText("progress.linux.starting_omarchy"), stageStart
	case "status.linux.desktop_starting":
		return uiText("progress.linux.loading_the_omarchy_desktop"), stageDesktop
	}
	return uiStatusText(key, values), stageOther
}

func linuxDownloadStatus(updating, systemImage bool) (string, linuxStage) {
	if systemImage {
		if updating {
			return uiText("progress.linux.downloading_the_omarchy_update"), stageDownload
		}
		return uiText("progress.linux.downloading_omarchy"), stageDownload
	}
	if updating {
		return uiText("progress.linux.getting_the_omarchy_update_ready"), stagePrepare
	}
	return uiText("progress.linux.getting_ready"), stagePrepare
}

// linuxProgressDetail is the line under the bar. A download says how much of
// it is done and that it survives an interruption; other steps let the bar speak.
func linuxProgressDetail(stage linuxStage, current, total int64) string {
	if stage == stageDesktop {
		return uiText("progress.linux.finish_first_time_account_setup_or_sign_in")
	}
	if stage == stageDownload && total > 0 {
		return uiTextWith("progress.linux.of_if_this_stops_it_continues_where_it", map[string]string{"current": linuxGB(current), "total": linuxGB(total)})
	}
	return ""
}
