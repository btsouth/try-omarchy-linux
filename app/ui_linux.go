//go:build linux

package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// progressUI mirrors status to the terminal and the optional GTK helper.
type progressUI struct {
	mu          sync.Mutex
	terminal    bool
	lastPercent int64
	midLine     bool
	window      *linuxSetupWindow
	state       linuxSetupState
	lastUpdate  time.Time
	booting     bool
}

var linuxUI = newLinuxProgressUI()

func newLinuxProgressUI() *progressUI {
	info, err := os.Stderr.Stat()
	return &progressUI{terminal: err == nil && info.Mode()&os.ModeCharDevice != 0, lastPercent: -1}
}

func getUI() *progressUI { return linuxUI }

func (u *progressUI) setStatus(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	logf("%s", msg)
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.midLine {
		fmt.Fprintln(os.Stderr)
		u.midLine = false
	}
	u.lastPercent = -1
	u.state = linuxSetupState{Status: msg, Booting: u.booting}
	if u.window != nil {
		u.window.update(u.state)
	}
	fmt.Fprintln(os.Stderr, msg)
}

func (u *progressUI) setBooting(booting bool) {
	u.mu.Lock()
	u.booting = booting
	u.state.Booting = booting
	if u.window != nil {
		u.window.update(u.state)
	}
	u.mu.Unlock()
}

func (u *progressUI) setProgress(current, total int64) {
	if total <= 0 {
		return
	}
	percent := min(current*100/total, 100)
	u.mu.Lock()
	defer u.mu.Unlock()
	u.state.Current, u.state.Total = current, total
	if u.window != nil && (time.Since(u.lastUpdate) >= 100*time.Millisecond || current >= total) {
		u.window.update(u.state)
		u.lastUpdate = time.Now()
	}
	step := int64(25)
	if u.terminal {
		step = 1
	}
	if u.lastPercent >= 0 && percent/step == u.lastPercent/step {
		return
	}
	u.lastPercent = percent
	if u.terminal {
		fmt.Fprintf(os.Stderr, "\r  %3d%%", percent)
		u.midLine = percent < 100
		if percent == 100 {
			fmt.Fprintln(os.Stderr)
		}
		return
	}
	fmt.Fprintf(os.Stderr, "  %d%%\n", percent)
}

func (u *progressUI) startWindow() {
	u.window = startLinuxSetupWindow()
	u.setStatus("Preparing Omarchy...")
}

func (u *progressUI) finish() {
	u.mu.Lock()
	w := u.window
	u.window = nil
	u.mu.Unlock()
	w.stop()
}

func (u *progressUI) showError(message string) {
	u.mu.Lock()
	w := u.window
	if w != nil {
		w.expected.Store(true)
		w.update(linuxSetupState{Status: message, Error: true})
	}
	u.mu.Unlock()
	if w != nil {
		select {
		case <-w.done:
		case <-setupContext().Done():
		}
	}
	u.finish()
}

// A slow desktop is a visible problem, but QEMU may still be usable. Keep the
// supervisor running while the user reads or dismisses this nonfatal message.
func (u *progressUI) showDesktopTimeout(message string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.state = linuxSetupState{Status: message, Error: true}
	if u.window != nil {
		u.window.expected.Store(true)
		u.window.update(u.state)
	}
	logf("%s", message)
	fmt.Fprintln(os.Stderr, message)
}
