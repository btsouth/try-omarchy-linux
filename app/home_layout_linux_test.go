//go:build linux

package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run in a private mount namespace with /home -> var/home. Ordinary hosts
// cannot create this system alias, so the integration runner opts in.
func TestLinuxAtomicHomePaths(t *testing.T) {
	if os.Getenv("TRYOMARCHY_ATOMIC_HOME_TEST") != "1" {
		t.Skip("requires the atomic-home mount namespace")
	}
	target, err := os.Readlink("/home")
	if err != nil || filepath.Clean(filepath.Join("/", target)) != "/var/home" {
		t.Fatalf("home alias: %q %v", target, err)
	}
	dir, err := platformQMPControlDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateMovePath(dir); err != nil {
		t.Fatal(err)
	}
	listener, err := listenLinuxCamera()
	if err != nil {
		t.Fatalf("private camera: %v", err)
	}
	defer listener.Close()
	camera, _ := linuxCameraSocketPath()
	for path, mode := range map[string]os.FileMode{dir: 0700, camera: 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("permissions: %s %v", path, err)
		}
	}
	if _, err := prepareQMPControl(); err != nil {
		t.Fatalf("private QMP: %v", err)
	}
	memory, cleanup, err := privateMemorySocket()
	if err != nil {
		t.Fatalf("saved-session socket: %v", err)
	}
	defer cleanup()
	ram, err := net.Listen("unix", memory)
	if err != nil {
		t.Fatal(err)
	}
	ram.Close()
	if err := validateMovePath("/home"); err == nil {
		t.Fatal("system alias accepted as a move target")
	}
	outside := "/var/home/outside"
	if err := os.MkdirAll(outside, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, filepath.Join(link, "disk.raw")} {
		if err := validateMovePath(path); err == nil || !strings.Contains(err.Error(), "linked paths") {
			t.Fatalf("unsafe link accepted: %s %v", path, err)
		}
	}
}

func TestLinuxAtomicHomeAncestor(t *testing.T) {
	for _, tc := range []struct {
		path, target string
		allowed      bool
	}{
		{"/home", "var/home", true},
		{"/home", "/var/home", true},
		{"/home", "/tmp/home", false},
		{"/home", "/var/home/other", false},
		{"/home/tester", "/var/home", false},
		{"/tmp/home", "/var/home", false},
	} {
		if got := linuxAtomicHomeAncestor(tc.path, tc.target); got != tc.allowed {
			t.Errorf("%q -> %q: %v", tc.path, tc.target, got)
		}
	}
}

func TestLinuxAtomicHomeRejectsLinkedDestination(t *testing.T) {
	if os.Getenv("TRYOMARCHY_ATOMIC_HOME_REJECT_TEST") != "1" {
		t.Skip("requires the unsafe atomic-home mount namespace")
	}
	if listener, err := listenLinuxCamera(); err == nil {
		listener.Close()
		t.Fatal("camera followed linked /var/home")
	}
	if _, err := prepareQMPControl(); err == nil {
		t.Fatal("QMP followed linked /var/home")
	}
	if _, cleanup, err := privateMemorySocket(); err == nil {
		cleanup()
		t.Fatal("saved-session followed linked /var/home")
	}
	entries, err := os.ReadDir("/outside/tester")
	if err != nil || len(entries) != 0 {
		t.Fatalf("unsafe destination changed: %v %v", entries, err)
	}
}
