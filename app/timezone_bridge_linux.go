//go:build linux

package main

import (
	"context"
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
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			// QEMU holds one connection; a reconnect replaces it.
			go func() {
				ticker := time.NewTicker(timeZoneInterval)
				defer ticker.Stop()
				_ = serveTimeZoneBridge(ctx, conn, zone, ticker.C)
			}()
		}
	}()
	return func() { cancel(); listener.Close() }, nil
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
