//go:build linux

package main

import (
	"fmt"
	"net/url"
	"strings"
)

// The Linux guest is built from the shared guest series plus the Linux patches
// (compatibility revision 43) and published on this repository's releases.
// Windows release defaults in manifest.go are deliberately untouched.
var (
	linuxGuestReleaseURL = "https://github.com/btsouth/try-omarchy-linux/releases/download/linux-v0.1.0"
	linuxGuestSumsSHA256 = "3ba8b875bc9ffff3dd223367a7c00e1e1e9d3b77a314bb147bba9b4361aeba73"
)

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
