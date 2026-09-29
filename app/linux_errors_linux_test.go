//go:build linux

package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestLinuxSetupFailuresSayWhatHappenedAndWhatToDo(t *testing.T) {
	t.Setenv("HOME", "/home/ana")
	dir := "/home/ana/.var/app/com.tryomarchy.TryOmarchy/data/try-omarchy"
	for _, tc := range []struct {
		name  string
		err   error
		title string
		want  []string
		help  string
	}{
		{"not enough space", fmt.Errorf("preflighting: %w", &insufficientSpaceError{path: dir + "/guest", need: 7<<30 + 300<<20, have: 2 << 30}),
			"Not enough space", []string{"This step needs 7.3 GB of free space in ~/.var/app/com.tryomarchy.TryOmarchy/data/try-omarchy", "only 2 GB is available", "downloaded so far is kept"}, "space"},
		{"disk full while writing", fmt.Errorf("writing: %w", syscall.ENOSPC),
			"The drive is full", []string{"Free some space, then try again", "downloaded so far is kept"}, "space"},
		{"no permission", fmt.Errorf("opening: %w", os.ErrPermission),
			"Try Omarchy cannot use that folder", []string{"not allowed to write to ~/.var/app/com.tryomarchy.TryOmarchy/data/try-omarchy"}, "storage"},
		{"drive gone", fmt.Errorf("reading: %w", os.ErrNotExist),
			"Storage is not available", []string{"If it is on a drive, reconnect it, then try again"}, "storage"},
		{"network", fmt.Errorf("manifest: %w", &url.Error{Op: "Get", URL: "https://example.test", Err: &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded}}),
			"Could not download Omarchy", []string{"Check your internet connection", "continues where it stopped"}, "downloads"},
		{"a stalled download", errors.New("download stalled for 1m30s"),
			"Could not download Omarchy", []string{"continues where it stopped"}, "downloads"},
		{"a checksum failure", errors.New("SHA256SUMS authentication failed"),
			"The download did not verify", []string{"did not match what was published", "not used"}, "downloads"},
		{"a missing release", errors.New("HTTP 404"),
			"Omarchy could not be found", []string{"not on the server", "check for an app update"}, "downloads"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := classifyLinuxSetupFailure(tc.err, dir)
			if f.Title != tc.title || f.Help != tc.help || !f.Retry {
				t.Fatalf("%+v", f)
			}
			for _, want := range tc.want {
				if !strings.Contains(f.Message, want) {
					t.Errorf("message %q lacks %q", f.Message, want)
				}
			}
			for _, jargon := range []string{"preflight", "errno", "ENOSPC", "GiB", "%w", "SHA256"} {
				if strings.Contains(f.Message, jargon) {
					t.Errorf("message leaks %q: %q", jargon, f.Message)
				}
			}
		})
	}
}

func TestLinuxUnknownFailureKeepsTheReasonForPeopleWhoReportIt(t *testing.T) {
	f := classifyLinuxSetupFailure(errors.New("something odd broke"), "/x")
	if f.Title != "Setting up Omarchy failed" || !strings.HasSuffix(f.Message, "something odd broke") || !f.Retry {
		t.Fatalf("%+v", f)
	}
}

func TestLinuxSetupFailureHelpStillNamesTheNextAction(t *testing.T) {
	if got := linuxSetupFailureHelp(fmt.Errorf("unpacking: %w", errInsufficientDiskSpace)); !strings.Contains(got, "Free some space") {
		t.Fatalf("%q", got)
	}
}
