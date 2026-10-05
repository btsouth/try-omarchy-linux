//go:build linux

package main

import (
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// linuxFailure is a setup failure in words: what happened, what to do, and
// whether trying again can help. The raw error stays in the log.
type linuxFailure struct {
	Title, Message, Help string
	Retry                bool
}

var linuxKeptNote = uiText("error.linux.what_you_have_downloaded_so_far_is_kept")

// linuxDataFolder names the folder a failure is about. Errors mention the
// guest or VM subfolder they were working in.
func linuxDataFolder(path string) string {
	path = filepath.Clean(path)
	if base := filepath.Base(path); base == "guest" || base == "vm" || base == "guest.next" {
		path = filepath.Dir(path)
	}
	return linuxDisplayPath(path)
}

func classifyLinuxSetupFailure(err error, dir string) linuxFailure {
	where := linuxDisplayPath(dir)
	var space *insufficientSpaceError
	switch {
	case errors.As(err, &space):
		return linuxFailure{Title: uiText("error.linux.not_enough_space"), Retry: true, Help: "space",
			Message: uiTextWith("error.linux.this_step_needs_of_free_space_in_and", map[string]string{"space_need": linuxGB(space.need), "path": linuxDataFolder(space.path), "space_have": linuxGB(space.have), "linux_kept_note": linuxKeptNote})}
	case errors.Is(err, errInsufficientDiskSpace) || isDiskFull(err):
		return linuxFailure{Title: uiText("error.linux.the_drive_is_full"), Retry: true, Help: "space",
			Message: uiTextWith("error.linux.the_drive_filled_up_while_setting_up_omarchy", map[string]string{"path": where, "linux_kept_note": linuxKeptNote})}
	case errors.Is(err, os.ErrPermission):
		return linuxFailure{Title: uiText("error.linux.try_omarchy_cannot_use_that_folder"), Retry: true, Help: "storage",
			Message: uiTextWith("error.linux.try_omarchy_is_not_allowed_to_write_to", map[string]string{"path": where})}
	case errors.Is(err, os.ErrNotExist):
		return linuxFailure{Title: uiText("error.linux.storage_is_not_available"), Retry: true, Help: "storage",
			Message: uiTextWith("error.linux.cannot_be_reached_if_it_is_on_a", map[string]string{"path": where})}
	}
	var urlErr *url.Error
	var netErr *net.OpError
	var dnsErr *net.DNSError
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "checksum mismatch") || strings.Contains(message, "authentication failed") || strings.Contains(message, "does not match"):
		return linuxFailure{Title: uiText("error.linux.the_download_did_not_verify"), Retry: true, Help: "downloads",
			Message: uiText("error.linux.try_omarchy_checks_every_file_it_downloads_this")}
	case strings.Contains(message, "http 404"):
		return linuxFailure{Title: uiText("error.linux.omarchy_could_not_be_found"), Retry: true, Help: "downloads",
			Message: uiText("error.linux.the_omarchy_files_for_this_version_are_not")}
	case errors.As(err, &urlErr) || errors.As(err, &netErr) || errors.As(err, &dnsErr) || errors.Is(err, syscall.ECONNRESET) || strings.Contains(message, "download stalled") || strings.Contains(message, "download failed after"):
		return linuxFailure{Title: uiText("error.linux.could_not_download_omarchy"), Retry: true, Help: "downloads",
			Message: uiText("error.linux.check_your_internet_connection_then_try_again_what")}
	}
	return linuxFailure{Title: uiText("error.linux.setting_up_omarchy_failed"), Retry: true,
		Message: uiTextWith("error.linux.something_went_wrong_while_setting_up_omarchy_in", map[string]string{"path": where, "error": err.Error()})}
}

// The launcher's own last words on the terminal and in the log, for people who
// run it there or send diagnostics.
func linuxSetupFailureHelp(err error) string {
	return classifyLinuxSetupFailure(err, ".").Message
}
