package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"sync/atomic"
	"time"
)

// Reclaim is also used by the launcher's local CLI. Only guest lifecycle
// messages can change readiness or reboot state, and those require QEMU.
func serveLifecycle(c net.Conn, fromQEMU func(net.Conn) bool, handle func(string, net.Conn)) {
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	// Capture ownership before reading: the guest sends one line and closes.
	trustedPeer := fromQEMU(c)
	line, err := bufio.NewReader(io.LimitReader(c, 64)).ReadString('\n')
	if err != nil {
		return
	}
	switch line {
	case "ready\n", "desktop-ready\n", "reboot\n", "shutdown\n":
		if !trustedPeer {
			return
		}
	case "reclaim\n":
	default:
		return
	}
	handle(line[:len(line)-1], c)
}

// The user-net lifecycle socket is not a reconnecting chardev. Hold an early
// connection briefly until the supervisor publishes the PID, never trusting a
// pending PID unless it becomes the current QEMU's exact PID.
func lifecyclePeerMatches(peer uint32, current func() uint32, wait func() bool) bool {
	if peer == 0 {
		return false
	}
	for {
		pid := current()
		if pid != 0 {
			return pid == peer
		}
		if !wait() {
			return false
		}
	}
}

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
		if activateRunningInstance() {
			os.Exit(0)
		}
		fatal(uiTextWith("fatal.port.lifecycle", map[string]string{"port": fmt.Sprint(lifecyclePort)}))
	}
	go func() {
		gate := make(chan struct{}, 4)
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			select {
			case gate <- struct{}{}:
				go func() {
					defer func() { <-gate }()
					serveLifecycle(c, lifecycleConnectionFromQEMU, func(line string, c net.Conn) {
						switch line {
						case "shutdown":
							guestCompositorHealth.stop()
						case "reboot":
							guestCompositorHealth.stop()
							logf("guest announced reboot")
							pendingReboot.Store(true)
						case "ready":
							logf("guest userspace announced ready")
							guestReady.Store(true)
							guestCompositorHealth.userspaceReady()
							guestDesktopReady()
						case "desktop-ready":
							desktopReady.Store(true)
						case "reclaim":
							c.SetWriteDeadline(time.Now().Add(3 * time.Second))
							if err := requestReclaimError(); err != nil {
								fmt.Fprintln(c, "error: "+err.Error())
							} else {
								fmt.Fprintln(c, "ok: "+reclaimStartedMessage())
							}
						}
					})
				}()
			default:
				c.Close()
			}
		}
	}()
}
