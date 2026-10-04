//go:build linux

package main

import (
	"bufio"
	"context"
	"errors"
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

type flakyListener struct {
	conns chan net.Conn
	fails int
}

func (l *flakyListener) Accept() (net.Conn, error) {
	if l.fails > 0 {
		l.fails--
		return nil, errors.New("too many open files")
	}
	conn, ok := <-l.conns
	if !ok {
		return nil, net.ErrClosed
	}
	return conn, nil
}
func (l *flakyListener) Close() error   { return nil }
func (l *flakyListener) Addr() net.Addr { return nil }

func TestLinuxTimeZoneBridgeReplacesConnectionsAndRecovers(t *testing.T) {
	listener := &flakyListener{conns: make(chan net.Conn), fails: 2}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		acceptTimeZoneConnections(ctx, listener, func() string { return "Etc/UTC" }, time.Hour, time.Millisecond)
		close(done)
	}()
	readOne := func(conn net.Conn) {
		t.Helper()
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if line, err := bufio.NewReader(conn).ReadString('\n'); err != nil || !strings.Contains(line, "Etc/UTC") {
			t.Fatalf("snapshot %q %v", line, err)
		}
	}
	firstHost, first := net.Pipe()
	listener.conns <- firstHost
	readOne(first)
	secondHost, second := net.Pipe()
	listener.conns <- secondHost
	readOne(second)
	// The replaced connection is closed by the bridge.
	first.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := first.Read(make([]byte, 1)); err == nil {
		t.Fatal("old connection stayed open")
	}
	close(listener.conns)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bridge did not stop when the listener closed")
	}
	second.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := second.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection stayed open after the bridge stopped")
	}
}

func TestLinuxTimeZoneBridgeNoticesAClosedPeerWithoutAZone(t *testing.T) {
	listener := &flakyListener{conns: make(chan net.Conn)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go acceptTimeZoneConnections(ctx, listener, func() string { return "" }, time.Hour, time.Millisecond)
	host, guest := net.Pipe()
	listener.conns <- host
	guest.Close()
	// net.Pipe reports the close to the bridge's reader; its side closes too.
	host.SetWriteDeadline(time.Now().Add(2 * time.Second))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := host.Write([]byte("x")); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("bridge kept a closed connection")
}
