//go:build linux

package main

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLinuxTimeZoneChannelUsesThePrivateSocket(t *testing.T) {
	cfg := config{followHostTimeZone: true, qmpDir: "/run/user/1000/TryOmarchyIPC"}
	args := buildQemuArgs(&cfg, "")
	if !slices.Contains(args, "socket,id=timezone0,path=/run/user/1000/TryOmarchyIPC/timezone.sock,reconnect-ms=1000") {
		t.Fatalf("time zone chardev missing or not private: %q", args)
	}
	for _, arg := range args {
		if strings.Contains(arg, "timezone0") && strings.Contains(arg, "host=") {
			t.Fatalf("time zone channel reachable over TCP: %s", arg)
		}
	}
}

func TestLinuxMonitorTimeZone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "timezone")
	if monitorTimeZone(path) != "" {
		t.Fatal("missing file gave a zone")
	}
	os.WriteFile(path, []byte("America/Louisville\n"), 0600)
	if got := monitorTimeZone(path); got != "America/Louisville" {
		t.Fatalf("zone = %q", got)
	}
	os.WriteFile(path, []byte("../etc/passwd\n"), 0600)
	if monitorTimeZone(path) != "" {
		t.Fatal("unsafe zone accepted")
	}
}

func TestLinuxTimeZoneBridgeServesItsSocket(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "tz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	before := qmpControlDirectory
	qmpControlDirectory = func() (string, error) { return filepath.Join(dir, "ipc"), nil }
	defer func() { qmpControlDirectory = before }()
	var zone atomic.Value
	zone.Store("Europe/Berlin")
	stop, err := startLinuxTimeZoneBridge(func() string { return zone.Load().(string) })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	path := filepath.Join(dir, "ipc", "timezone.sock")
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("socket %v %v", info, err)
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || line != "{\"type\":\"timezone\",\"zone\":\"Europe/Berlin\"}\n" {
		t.Fatalf("snapshot %q %v", line, err)
	}
	if _, err := startLinuxTimeZoneBridge(func() string { return "Etc/UTC" }); err == nil {
		t.Fatal("a second bridge took over a live socket")
	}
}
