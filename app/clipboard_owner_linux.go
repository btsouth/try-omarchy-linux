//go:build linux

package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// linuxSelectionOwner runs the process that serves the host clipboard after
// Omarchy copies to it. xclip and wl-copy normally fork into the background
// to keep serving the selection, and that detached copy outlived the app: it
// held the sandbox open until something else was copied. Run in the
// foreground instead, each owner stays a child the bridge can stop.
type linuxSelectionOwner struct {
	mu      sync.Mutex
	current *linuxOwnerProcess
	closed  bool
}

type linuxOwnerProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
}

var (
	// How long an owner may take to report that it holds the selection.
	linuxSelectionOwnerTimeout = 3 * time.Second
	// wl-copy reports nothing once it holds the selection, so a foreground
	// wl-copy still running after this long is taken to be serving it.
	linuxWaylandOwnerSettle = 250 * time.Millisecond
)

// copy makes data the host clipboard as mime, replacing the previous owner.
func (o *linuxSelectionOwner) copy(mime string, data []byte) bool {
	o.mu.Lock()
	closed := o.closed
	o.mu.Unlock()
	if closed {
		return false
	}
	wayland := os.Getenv("WAYLAND_DISPLAY") != ""
	cmd := exec.Command("wl-copy", "--foreground", "--type", mime)
	if !wayland {
		cmd = exec.Command("xclip", "-quiet", "-selection", "clipboard", "-in", "-target", mime)
	}
	cmd.Stdin = bytes.NewReader(data)
	// A pipe of our own rather than StderrPipe, so reading it never races
	// with Wait in the reaper below.
	stderr, stderrWriter, err := os.Pipe()
	if err != nil {
		logf("clipboard owner: %v", err)
		return false
	}
	cmd.Stderr = stderrWriter
	if err := cmd.Start(); err != nil {
		stderr.Close()
		stderrWriter.Close()
		logf("clipboard owner: %v", err)
		return false
	}
	stderrWriter.Close()
	owner := &linuxOwnerProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		cmd.Wait()
		close(owner.done)
	}()

	// xclip -quiet says this once it owns the selection, then waits.
	ready := make(chan struct{})
	go func() {
		defer stderr.Close()
		lines := bufio.NewScanner(stderr)
		signalled := false
		for lines.Scan() {
			if !signalled && !wayland && strings.HasPrefix(lines.Text(), "Waiting for") {
				close(ready)
				signalled = true
			}
		}
		io.Copy(io.Discard, stderr)
	}()
	var settle <-chan time.Time
	if wayland {
		settle = time.After(linuxWaylandOwnerSettle)
	}
	ok := false
	select {
	case <-ready:
		ok = true
	case <-settle:
		ok = true
	case <-owner.done:
		// Only a foreground wl-copy ends cleanly once it has served its
		// selection, when something else replaced it straight away.
		ok = wayland && cmd.ProcessState.Success()
	case <-time.After(linuxSelectionOwnerTimeout):
		logf("clipboard owner: %s did not take the selection in time", cmd.Path)
	}
	if !ok {
		owner.stop()
		return false
	}

	o.mu.Lock()
	previous := o.current
	if o.closed {
		o.mu.Unlock()
		owner.stop()
		return false
	}
	o.current = owner
	o.mu.Unlock()
	// The new owner took the selection, so the previous one serves nothing.
	previous.stop()
	return true
}

// close stops the current owner. The host clipboard keeps what Omarchy last
// copied only while the app runs, as with any X11 or Wayland application.
func (o *linuxSelectionOwner) close() {
	o.mu.Lock()
	o.closed = true
	owner := o.current
	o.current = nil
	o.mu.Unlock()
	owner.stop()
}

func (p *linuxOwnerProcess) stop() {
	if p == nil {
		return
	}
	select {
	case <-p.done:
		return
	default:
	}
	p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		p.cmd.Process.Kill()
		<-p.done
	}
}
