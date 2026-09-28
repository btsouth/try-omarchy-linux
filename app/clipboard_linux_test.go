//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestLinuxClipboardMIME(t *testing.T) {
	for _, tt := range []struct{ types, want string }{
		{"text/plain\nimage/png\n", "image/png"},
		{"text/uri-list\napplication/vnd.portal.filetransfer", "application/vnd.portal.filetransfer"},
		{"text/plain\ntext/uri-list\nimage/png", "text/uri-list"},
		{"text/plain\nx-kde-passwordManagerHint", ""},
		{"application/octet-stream", ""},
		{"text/plain;charset=utf-8\ntext/plain", "text/plain;charset=utf-8"},
	} {
		if got := linuxClipboardMIME(tt.types); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.types, got, tt.want)
		}
	}
}

func TestLinuxClipboardFileURIs(t *testing.T) {
	paths, ok := linuxClipboardPaths([]byte("# comment\r\nfile:///tmp/caf%C3%A9%20one.txt\r\nfile://localhost/tmp/two\r\n"))
	if !ok || len(paths) != 2 || paths[0] != "/tmp/café one.txt" || paths[1] != "/tmp/two" {
		t.Fatalf("%v %v", paths, ok)
	}
	for _, bad := range []string{"https://example.org/a", "file://remote/tmp/a", "file:relative", "file:///tmp/a%00b", "file:///tmp/a?x=y", "file:///tmp/a#b", "file://user@localhost/tmp/a", "file:///tmp/%ZZ", strings.Repeat("file:///a\n", 1001)} {
		if _, ok := linuxClipboardPaths([]byte(bad)); ok {
			t.Errorf("accepted invalid URI list %q", bad[:min(len(bad), 80)])
		}
	}
}
