//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClipboardTool installs a script named name on PATH that saves its input
// and arguments, then runs body.
func fakeClipboardTool(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" > \"" + dir + "/args\"\ncat > \"" + dir + "/input\"\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func ownerRunning(o *linuxSelectionOwner) *linuxOwnerProcess {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.current
}

func exited(p *linuxOwnerProcess) bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func stopped(p *linuxOwnerProcess) bool {
	select {
	case <-p.done:
		return true
	case <-time.After(2 * time.Second):
		return false
	}
}

func TestLinuxSelectionOwnerKeepsXclipUntilClosed(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	dir := fakeClipboardTool(t, "xclip", "echo 'Waiting for selection requests, Control-C to quit' >&2\nexec sleep 30")
	var o linuxSelectionOwner
	if !o.copy("text/plain;charset=utf-8", []byte("hello")) {
		t.Fatal("copy failed")
	}
	first := ownerRunning(&o)
	if first == nil || exited(first) {
		t.Fatal("xclip should keep serving the selection in the foreground")
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if !strings.HasPrefix(string(args), "-quiet -selection clipboard -in") {
		t.Fatalf("xclip must run in the foreground, got args %q", args)
	}
	if input, _ := os.ReadFile(filepath.Join(dir, "input")); string(input) != "hello" {
		t.Fatalf("xclip got %q", input)
	}
	if !o.copy("text/plain;charset=utf-8", []byte("again")) {
		t.Fatal("second copy failed")
	}
	if !stopped(first) {
		t.Fatal("the replaced owner kept running")
	}
	second := ownerRunning(&o)
	o.close()
	if !stopped(second) {
		t.Fatal("close left the owner running")
	}
	if o.copy("text/plain;charset=utf-8", []byte("late")) {
		t.Fatal("copy succeeded after close")
	}
}

func TestLinuxSelectionOwnerReportsXclipFailure(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	fakeClipboardTool(t, "xclip", "echo 'xclip: error: Failed to take ownership of selection.' >&2\nexit 1")
	var o linuxSelectionOwner
	if o.copy("text/plain;charset=utf-8", []byte("hello")) {
		t.Fatal("copy reported success when xclip failed")
	}
	if ownerRunning(&o) != nil {
		t.Fatal("a failed owner was kept")
	}
}

func TestLinuxSelectionOwnerStopsSilentXclip(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	defer func(d time.Duration) { linuxSelectionOwnerTimeout = d }(linuxSelectionOwnerTimeout)
	linuxSelectionOwnerTimeout = 200 * time.Millisecond
	dir := fakeClipboardTool(t, "xclip", `echo $$ > "$(dirname "$0")/pid"`+"\nexec sleep 30")
	var o linuxSelectionOwner
	if o.copy("text/plain;charset=utf-8", []byte("hello")) {
		t.Fatal("copy succeeded without xclip taking the selection")
	}
	pid, _ := os.ReadFile(filepath.Join(dir, "pid"))
	if len(pid) == 0 {
		t.Fatal("fake xclip did not start")
	}
	if _, err := os.Stat("/proc/" + strings.TrimSpace(string(pid))); err == nil {
		t.Fatal("xclip that never took the selection was left running")
	}
}

func TestLinuxSelectionOwnerRunsWlCopyInForeground(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	dir := fakeClipboardTool(t, "wl-copy", "exec sleep 30")
	var o linuxSelectionOwner
	if !o.copy("image/png", []byte("png")) {
		t.Fatal("copy failed")
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if strings.TrimSpace(string(args)) != "--foreground --type image/png" {
		t.Fatalf("wl-copy must run in the foreground, got args %q", args)
	}
	owner := ownerRunning(&o)
	o.close()
	if owner == nil || !stopped(owner) {
		t.Fatal("close left wl-copy running")
	}
}

func TestLinuxSelectionOwnerReportsWlCopyFailure(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	fakeClipboardTool(t, "wl-copy", "exit 1")
	var o linuxSelectionOwner
	if o.copy("text/plain;charset=utf-8", []byte("hello")) {
		t.Fatal("copy reported success when wl-copy failed")
	}
}
