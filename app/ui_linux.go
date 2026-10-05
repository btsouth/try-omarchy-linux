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
	stage       linuxStage
	updating    bool
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
	text, stage := linuxFriendlyStatus(msg, u.updating)
	u.stage = stage
	u.state = linuxSetupState{Status: text, Detail: linuxProgressDetail(stage, 0, 0), Booting: u.booting}
	if u.window != nil {
		u.window.update(u.state)
	}
	fmt.Fprintln(os.Stderr, msg)
}

// setUpdating names the guest download an update, so its steps say so.
func (u *progressUI) setUpdating(updating bool) {
	u.mu.Lock()
	u.updating = updating
	u.mu.Unlock()
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
	// Small preparation steps finish in moments; a bar filling for each one
	// says nothing about how far along setup is.
	if u.stage != stagePrepare {
		u.state.Current, u.state.Total = current, total
		u.state.Detail = linuxProgressDetail(u.stage, current, total)
	}
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

// showFailure puts a setup failure on the window and waits for the answer.
// It reports whether the person asked to try again. Without a window it only
// reports the failure on the terminal.
func (u *progressUI) showFailure(f linuxFailure) (retry bool) {
	u.mu.Lock()
	w := u.window
	u.mu.Unlock()
	if w == nil {
		return false
	}
	state := linuxSetupState{Prompt: "error", ErrorTitle: f.Title, Status: f.Message, CanRetry: f.Retry}
	if f.Help != "" || f.Title != "" {
		state.HelpURL = linuxHelpURL(f.Help)
	}
	answer, err := w.ask(setupContext(), state)
	return err == nil && answer == "retry"
}

// Account setup or login can take as long as the person needs. Keep the
// supervisor and stop action available without reporting a running VM as failed.
func (u *progressUI) showDesktopTimeout(message string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.state = linuxSetupState{Status: uiText("status.linux.waiting_for_the_omarchy_desktop"), Detail: message, Booting: true}
	if u.window != nil {
		u.window.update(u.state)
	}
	logf("%s", message)
	fmt.Fprintln(os.Stderr, message)
}
