//go:build linux

package main

import (
	"fmt"
	"os"
	"sync"
)

// progressUI is the Linux stand-in for the Windows setup window until the
// Linux front end has its own. Status lines go to the terminal and the log.
// Progress redraws one line on a terminal and prints every 25 percent
// otherwise.
type progressUI struct {
	mu          sync.Mutex
	terminal    bool
	lastPercent int64
	midLine     bool
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
	fmt.Fprintln(os.Stderr, msg)
}

func (u *progressUI) setProgress(current, total int64) {
	if total <= 0 {
		return
	}
	percent := min(current*100/total, 100)
	u.mu.Lock()
	defer u.mu.Unlock()
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
