package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

var (
	pendingReboot atomic.Bool
	guestReady    atomic.Bool
	desktopReady  atomic.Bool
)

// runLifecycleListener receives the guest's shutdown intent: the image's
// try-omarchy-reboot-notify unit connects to 10.0.2.2:4450 (this listener via
// user-net) and says "reboot" when the guest is rebooting rather than
// powering off.
func runLifecycleListener() {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", lifecyclePort))
	if err != nil {
		fatal("Try Omarchy looks like it's already running (port %d is in use).", lifecyclePort)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				c.SetReadDeadline(time.Now().Add(3 * time.Second))
				line, err := bufio.NewReader(io.LimitReader(c, 64)).ReadString('\n')
				if err != nil {
					return
				}
				switch strings.TrimSpace(line) {
				case "reboot":
					logf("guest announced reboot")
					pendingReboot.Store(true)
				case "ready":
					logf("guest userspace announced ready")
					guestReady.Store(true)
				case "desktop-ready":
					logf("guest desktop announced ready")
					desktopReady.Store(true)
				case "reclaim":
					c.SetWriteDeadline(time.Now().Add(3 * time.Second))
					if err := requestReclaimError(); err != nil {
						fmt.Fprintln(c, "error: "+err.Error())
					} else {
						fmt.Fprintln(c, "ok: Preparing free space. Check Reclaim status in the tray before shutting down.")
					}
				}
			}(c)
		}
	}()
}
