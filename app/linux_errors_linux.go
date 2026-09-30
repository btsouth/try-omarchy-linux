//go:build linux

package main

import (
	"errors"
	"fmt"
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

const linuxKeptNote = "What you have downloaded so far is kept."

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
		return linuxFailure{Title: "Not enough space", Retry: true, Help: "space",
			Message: fmt.Sprintf("This step needs %s of free space in %s, and only %s is available. Free some space, then try again. %s", linuxGB(space.need), linuxDataFolder(space.path), linuxGB(space.have), linuxKeptNote)}
	case errors.Is(err, errInsufficientDiskSpace) || isDiskFull(err):
		return linuxFailure{Title: "The drive is full", Retry: true, Help: "space",
			Message: "The drive filled up while setting up Omarchy in " + where + ". Free some space, then try again. " + linuxKeptNote}
	case errors.Is(err, os.ErrPermission):
		return linuxFailure{Title: "Try Omarchy cannot use that folder", Retry: true, Help: "storage",
			Message: "Try Omarchy is not allowed to write to " + where + ". Fix the folder's permissions, then try again."}
	case errors.Is(err, os.ErrNotExist):
		return linuxFailure{Title: "Storage is not available", Retry: true, Help: "storage",
			Message: where + " cannot be reached. If it is on a drive, reconnect it, then try again."}
	}
	var urlErr *url.Error
	var netErr *net.OpError
	var dnsErr *net.DNSError
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "checksum mismatch") || strings.Contains(message, "authentication failed") || strings.Contains(message, "does not match"):
		return linuxFailure{Title: "The download did not verify", Retry: true, Help: "downloads",
			Message: "Try Omarchy checks every file it downloads. This one did not match what was published, so it was not used. Try again to download it fresh. If it keeps failing, see the help page."}
	case strings.Contains(message, "http 404"):
		return linuxFailure{Title: "Omarchy could not be found", Retry: true, Help: "downloads",
			Message: "The Omarchy files for this version are not on the server. Try again later, and check for an app update."}
	case errors.As(err, &urlErr) || errors.As(err, &netErr) || errors.As(err, &dnsErr) || errors.Is(err, syscall.ECONNRESET) || strings.Contains(message, "download stalled") || strings.Contains(message, "download failed after"):
		return linuxFailure{Title: "Could not download Omarchy", Retry: true, Help: "downloads",
			Message: "Check your internet connection, then try again. What has been downloaded is kept, so it continues where it stopped."}
	}
	return linuxFailure{Title: "Setting up Omarchy failed", Retry: true,
		Message: "Something went wrong while setting up Omarchy in " + where + ". Try again. If it keeps happening, open Backup and recovery or see the help page.\n\n" + err.Error()}
}

// The launcher's own last words on the terminal and in the log, for people who
// run it there or send diagnostics.
func linuxSetupFailureHelp(err error) string {
	return classifyLinuxSetupFailure(err, ".").Message
}
