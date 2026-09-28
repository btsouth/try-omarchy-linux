//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestLinuxDefaultDoesNotSelectWindowsGuest(t *testing.T) {
	release, pin, err := selectLinuxGuestRelease("", "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if release == defaultReleaseURL || pin == defaultSumsSHA256 {
		t.Fatal("Linux ordinary launch selected Windows guest defaults")
	}
	if release != "http://127.0.0.1:18090" || pin != "bc783fcb5b83359e6cde52f7e5bb640bacb3adf747c487bbe9bacfb0c5171826" {
		t.Fatalf("unexpected private Linux guest pair: %s %s", release, pin)
	}
}

func TestLinuxReleaseOverrideRequiresAnAuthenticatedPair(t *testing.T) {
	for _, tc := range []struct {
		url, pin       string
		urlSet, pinSet bool
	}{
		{url: "https://example.test/release", urlSet: true},
		{pin: strings.Repeat("a", 64), pinSet: true},
		{url: "http://example.test/release", pin: strings.Repeat("a", 64), urlSet: true, pinSet: true},
		{url: "https://example.test/release", pin: "bad", urlSet: true, pinSet: true},
	} {
		if _, _, err := selectLinuxGuestRelease(tc.url, tc.pin, tc.urlSet, tc.pinSet); err == nil {
			t.Fatalf("accepted unsafe or incomplete override: %+v", tc)
		}
	}
	gotURL, gotPin, err := selectLinuxGuestRelease("https://example.test/release/", strings.Repeat("A", 64), true, true)
	if err != nil || gotURL != "https://example.test/release" || gotPin != strings.Repeat("a", 64) {
		t.Fatalf("authenticated override: %s %s %v", gotURL, gotPin, err)
	}
}

func TestLinuxReleaseTagsNameTheirVersion(t *testing.T) {
	if got := releaseVersion("https://github.com/btsouth/try-omarchy-linux/releases/download/linux-v0.1.0"); got != "v0.1.0" {
		t.Fatalf("linux release version: %s", got)
	}
	if got := releaseVersion("http://127.0.0.1:18090"); got != currentVersion {
		t.Fatalf("untagged release version: %s", got)
	}
}
