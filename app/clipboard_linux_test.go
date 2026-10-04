//go:build linux

package main

import (
	"os"
	"path/filepath"
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

func TestLinuxClipboardOffDisablesSyncOnEveryBackend(t *testing.T) {
	for _, desktop := range []struct{ name, wayland, display string }{
		{"GNOME", "wayland-test", ""},
		{"KDE", "wayland-test", ""},
		{"X11", "", ":test"},
	} {
		t.Run(desktop.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("WAYLAND_DISPLAY", desktop.wayland)
			t.Setenv("DISPLAY", desktop.display)
			t.Setenv("XDG_CURRENT_DESKTOP", desktop.name)
			bin := t.TempDir()
			called := filepath.Join(bin, "called")
			t.Setenv("PATH", bin)
			t.Setenv("TRYOMARCHY_CLIPBOARD_PROBE", called)
			for _, name := range []string{"xclip", "wl-copy", "wl-paste", "try-omarchy-clipboard-capabilities"} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf called >> \"$TRYOMARCHY_CLIPBOARD_PROBE\"\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := setLinuxClipboardSharing(false); err != nil {
				t.Fatal(err)
			}
			stop := runLinuxClipboardBridge()
			defer stop()
			b := desktopClipboard.Load()
			if b == nil {
				t.Fatal("sharing off must keep the file-drop transport available")
			}
			if b.sequence != nil || b.getPaths != nil || b.setPaths != nil || b.setDropPaths != nil || b.setHost(textItem("guest text")) {
				t.Fatal("sharing off must disable host and guest clipboard access")
			}
			if b.transfers == nil || b.dropRequests == nil {
				t.Fatal("sharing off must preserve explicit file drops")
			}
			if _, err := os.Stat(called); !os.IsNotExist(err) {
				t.Fatalf("sharing off invoked a clipboard utility: %v", err)
			}
			if got := linuxClipboardStatus.Load(); got != linuxClipboardOffMessage {
				t.Fatalf("status = %q, want %q", got, linuxClipboardOffMessage)
			}
		})
	}
}
