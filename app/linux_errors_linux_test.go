//go:build linux

package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestLinuxSetupErrorsOfferRelevantNextAction(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("unpacking: %w", errInsufficientDiskSpace), "Free space"},
		{fmt.Errorf("opening shared folder: %w", os.ErrPermission), "access"},
		{fmt.Errorf("reading saved folder: %w", os.ErrNotExist), "Reconnect"},
		{fmt.Errorf("manifest: %w", &url.Error{Op: "Get", URL: "https://example.test", Err: &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded}}), "source could not be reached"},
		{fmt.Errorf("SHA256SUMS authentication failed"), "failed verification"},
		{fmt.Errorf("HTTP 404"), "unavailable"},
	} {
		if got := linuxSetupFailureHelp(tc.err); !strings.Contains(got, tc.want) {
			t.Errorf("%v: %q does not contain %q", tc.err, got, tc.want)
		}
	}
}
