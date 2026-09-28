//go:build linux

package main

import (
	"fmt"
	"net/url"
	"strings"
)

// This local engineering build uses the Phase 8B Linux guest, rebuilt from
// the Phase 6 base with patch 0101, through a private loopback fixture.
// A public build needs an approved Linux release and trust root.
// Windows release defaults in manifest.go are deliberately untouched.
var (
	linuxGuestReleaseURL = "http://127.0.0.1:18090"
	linuxGuestSumsSHA256 = "bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826"
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
