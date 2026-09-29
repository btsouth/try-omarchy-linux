//go:build linux

package main

import (
	"fmt"
	"net/url"
	"strings"
)

// The Linux guest is built from the shared guest series plus the Linux patches
// (compatibility revision 44) and published on this repository's releases.
// Windows release defaults in manifest.go are deliberately untouched.
var (
	linuxGuestReleaseURL = "https://github.com/btsouth/try-omarchy-linux/releases/download/linux-v0.1.1"
	linuxGuestSumsSHA256 = "e0c5113c0eb9fa489bae3fc0c0abdf5696c35515f6b80db0660d4d3492d4a694"
)

// Measured on the pinned guest image (linux-v0.1.1): the archive, kernel and
// initramfs are 2.03 GB together, and a first launch ends with 5.7 GB of system
// files, the VM's own 5.7 GB copy of them and about 1.4 GB the first boot
// writes. Update these with the pin; the release manifest lists the sizes.
const (
	linuxGuestDownloadBytes = 2_177_000_000
	linuxGuestSpaceBytes    = int64(13) << 30
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
		return "", "", fmt.Errorf("a custom Omarchy image needs both -release and -sums-sha256")
	}
	if !releaseExplicit {
		release, sumsSHA256 = linuxGuestReleaseURL, linuxGuestSumsSHA256
	}
	release = normalizedRelease(release)
	sumsSHA256 = normalizedSHA256(sumsSHA256)
	if !validSHA256(sumsSHA256) {
		return "", "", fmt.Errorf("the Linux Omarchy image has no valid trusted SHA256SUMS digest")
	}
	u, err := url.Parse(release)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", fmt.Errorf("the Linux Omarchy image URL is invalid")
	}
	if u.Scheme == "http" && !strings.EqualFold(u.Hostname(), "localhost") && u.Hostname() != "127.0.0.1" && u.Hostname() != "[::1]" && u.Hostname() != "::1" {
		return "", "", fmt.Errorf("the Linux Omarchy image needs HTTPS outside local test fixtures")
	}
	return release, sumsSHA256, nil
}
