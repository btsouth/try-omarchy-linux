package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"
)

const timeZoneInterval = 5 * time.Second
const timeZoneDevice = "dev.tryomarchy.timezone"

func guestAcceptsTimeZone(spec buildSpec) bool {
	return slices.Contains(spec.Runtime.OptionalDevices, timeZoneDevice)
}

func hostTimeZoneSnapshot(zone string) []byte {
	if !validZoneName.MatchString(zone) || strings.Contains(zone, "..") {
		return nil
	}
	message, _ := json.Marshal(struct {
		Type string `json:"type"`
		Zone string `json:"zone"`
	}{"timezone", zone})
	return append(message, '\n')
}

// QEMU connects a dedicated root-only guest virtio port to the host listener.
// Periodic snapshots cover guest boot, reconnect, host wake, and zone changes.
// It does not read guest commands or change either machine's clock.
func serveTimeZoneBridge(ctx context.Context, conn net.Conn, zone func() string, ticks <-chan time.Time) error {
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	for {
		if message := hostTimeZoneSnapshot(zone()); message != nil {
			if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return err
			}
			for len(message) > 0 {
				n, err := conn.Write(message)
				if err != nil {
					return err
				}
				if n <= 0 {
					return fmt.Errorf("time zone channel stopped accepting data")
				}
				message = message[n:]
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-ticks:
			if !ok {
				return nil
			}
		}
	}
}
