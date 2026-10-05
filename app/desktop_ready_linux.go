//go:build linux

package main

import (
	"context"
	"os"
	"time"
)

type linuxDesktopWaitResult int

// The same prompt owns a close answer before and after desktop handoff.
// Its context is cancelled when QEMU exits, including while a dialog is open.
type linuxShutdownConfirmation struct {
	ctx        context.Context
	cancel     context.CancelFunc
	pending    <-chan bool
	open       func(context.Context) <-chan bool
	openForce  func(context.Context) <-chan bool
	force      bool
	shutdownAt time.Time
}

func newLinuxShutdownConfirmation(open func(context.Context) <-chan bool) *linuxShutdownConfirmation {
	ctx, cancel := context.WithCancel(context.Background())
	return &linuxShutdownConfirmation{ctx: ctx, cancel: cancel, open: open, openForce: confirmLinuxForceStop}
}

func (c *linuxShutdownConfirmation) request() {
	if c != nil && c.pending == nil && c.open != nil {
		c.pending = c.open(c.ctx)
	}
}

var linuxShutdownGracePeriod = 15 * time.Second

func (c *linuxShutdownConfirmation) requestShutdown(requests int, now time.Time) {
	if c.pending != nil {
		return
	}
	if requests == 0 {
		c.force = false
		c.request()
	} else if requests == 1 && !c.shutdownAt.IsZero() && now.Sub(c.shutdownAt) >= linuxShutdownGracePeriod && c.openForce != nil {
		if c.ctx.Err() != nil {
			c.ctx, c.cancel = context.WithCancel(context.Background())
		}
		c.force = true
		c.pending = c.openForce(c.ctx)
	}
}

func (c *linuxShutdownConfirmation) close() {
	if c != nil {
		c.cancel()
		c.pending = nil
	}
}

func (c *linuxShutdownConfirmation) answer() <-chan bool {
	if c == nil {
		return nil
	}
	return c.pending
}

const (
	linuxDesktopReady linuxDesktopWaitResult = iota
	linuxDesktopTimedOut
	linuxDesktopExited
	linuxDesktopCancelled
	linuxDesktopInterrupted
)

const linuxDesktopReadyTimeout = 5 * time.Minute

// Keep consuming QMP events while the guest boots. Its control socket can be
// ready well before the login session and Omarchy shell are on screen.
func waitLinuxDesktopReady(ctx context.Context, exited <-chan error, stop <-chan os.Signal,
	lines <-chan string, visibility *linuxVisibility, ready, userspace func() bool,
	timeout, interval time.Duration, status func(string), confirmation *linuxShutdownConfirmation) linuxDesktopWaitResult {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	userspaceReported := false
	for {
		// Cancellation and QEMU exit win if either arrives with a late
		// desktop announcement. Never leave a cancelled VM running.
		select {
		case <-ctx.Done():
			return linuxDesktopCancelled
		case <-stop:
			return linuxDesktopInterrupted
		case <-exited:
			return linuxDesktopExited
		default:
		}
		if ready() {
			return linuxDesktopReady
		}
		select {
		case <-ctx.Done():
			return linuxDesktopCancelled
		case <-stop:
			return linuxDesktopInterrupted
		case <-exited:
			return linuxDesktopExited
		case <-deadline.C:
			return linuxDesktopTimedOut
		case confirmed := <-confirmation.answer():
			confirmation.pending = nil
			if confirmed {
				return linuxDesktopInterrupted
			}
		case line, ok := <-lines:
			if !ok {
				lines = nil
				continue
			}
			if visibility.receive(line, time.Now()) {
				sendLinuxVisibility(visibility)
			}
			if closeRequested(line) {
				confirmation.request()
			}
		case <-tick.C:
			if !userspaceReported && userspace() {
				userspaceReported = true
				status(uiText("status.linux.desktop_starting"))
			}
		}
	}
}
