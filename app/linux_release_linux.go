//go:build linux

package main

import (
	"net/url"
	"strings"
)

// The Linux guest is built from the shared guest series plus the Linux patches
// (compatibility revision 62) and published on this repository's releases.
// Windows release defaults in manifest.go are deliberately untouched.
var (
	linuxGuestReleaseURL = "https://github.com/btsouth/try-omarchy-linux/releases/download/linux-v0.3.0"
	linuxGuestSumsSHA256 = "f101b139d441a2b27b9ac9d32d4e2abc626e11636a755255b344039c384fd80e"
)

// Measured on the pinned guest image (linux-v0.3.0): the archive, kernel and
// initramfs total 2,285,052,324 bytes (2.13 GiB). The sparse factory image uses
// about 5.86 GiB before boot.
// Allow 14 GiB for setup, including the kernel, initramfs and room for writes.
// Update these measurements with the pin; the release manifest lists the sizes.
const (
	linuxGuestDownloadBytes = 2_285_052_324
	linuxGuestSpaceBytes    = int64(14) << 30
	// A sparse VM disk can grow past what its drive has left. Below this the
	// guest can hit write errors, so the home says so.
	linuxLowSpaceBytes = int64(2) << 30
)

// The public help page. Errors and banners link to its sections.
const linuxHelpPage = "https://github.com/btsouth/try-omarchy-linux/blob/master/docs/LINUX-HELP.md"

func linuxHelpURL(section string) string {
	if section == "" {
		return linuxHelpPage
	}
	return linuxHelpPage + "#" + section
}

func selectLinuxGuestRelease(release, sumsSHA256 string, releaseExplicit, sumsExplicit bool) (string, string, error) {
	if releaseExplicit != sumsExplicit {
		return "", "", uiError(uiText("error.linux.custom_image_flags"), nil)
	}
	if !releaseExplicit {
		release, sumsSHA256 = linuxGuestReleaseURL, linuxGuestSumsSHA256
	}
	release = normalizedRelease(release)
	sumsSHA256 = normalizedSHA256(sumsSHA256)
	if !validSHA256(sumsSHA256) {
		return "", "", uiError(uiText("error.linux.image_digest"), nil)
	}
	u, err := url.Parse(release)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", uiError(uiText("error.linux.image_url"), nil)
	}
	if u.Scheme == "http" && !strings.EqualFold(u.Hostname(), "localhost") && u.Hostname() != "127.0.0.1" && u.Hostname() != "[::1]" && u.Hostname() != "::1" {
		return "", "", uiError(uiText("error.linux.image_https"), nil)
	}
	return release, sumsSHA256, nil
}
