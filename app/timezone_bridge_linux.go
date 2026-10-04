//go:build linux

package main

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// timeZoneChardev uses a private filesystem socket, like the camera, so the
// guest cannot reach the bridge through QEMU's user network.
func timeZoneChardev(dir string) string {
	return "socket,id=timezone0,path=" + qemuOptionValue(filepath.Join(dir, "timezone.sock")) + ",reconnect-ms=1000"
}

// startLinuxTimeZoneBridge must succeed before QEMU starts: QEMU 11.1 aborts
// when a reconnecting client chardev finds nothing listening.
func startLinuxTimeZoneBridge(zone func() string) (func(), error) {
	listener, err := listenLinuxPrivateSocket("timezone.sock")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	go acceptTimeZoneConnections(ctx, listener, zone, timeZoneInterval, time.Second)
	return func() { cancel(); listener.Close() }, nil
}

// acceptTimeZoneConnections serves one connection at a time: QEMU holds one,
// and a reconnect replaces it. A failed accept, such as running out of file
// descriptors, is retried so a later QEMU reconnect is still served.
func acceptTimeZoneConnections(ctx context.Context, listener net.Listener, zone func() string, interval, retry time.Duration) {
	var current context.CancelFunc
	defer func() {
		if current != nil {
			current()
		}
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			logf("time zone: accept: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(retry):
			}
			continue
		}
		if current != nil {
			current()
		}
		connCtx, connCancel := context.WithCancel(ctx)
		current = connCancel
		go func() {
			defer connCancel()
			// The guest never writes. A read ends when the peer closes, even
			// while there is no valid zone to send.
			go func() {
				_, _ = io.Copy(io.Discard, conn)
				connCancel()
			}()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			_ = serveTimeZoneBridge(connCtx, conn, zone, ticker.C)
		}()
	}
}

// linuxMonitorTimeZone is kept current by Flatpak's session helper. Inside
// the sandbox /etc/localtime is fixed when the app starts.
var linuxMonitorTimeZone = "/run/host/monitor/timezone"

// liveHostTimeZone reads this computer's current zone.
func liveHostTimeZone() string {
	if inFlatpak() {
		if zone := monitorTimeZone(linuxMonitorTimeZone); zone != "" {
			return zone
		}
	}
	return hostTimeZone()
}

func monitorTimeZone(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if zone := strings.TrimSpace(string(data)); validZoneName.MatchString(zone) {
		return zone
	}
	return ""
}
