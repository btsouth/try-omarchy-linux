//go:build linux

package main

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// Local port forwards saved in Settings while Omarchy runs change the running
// VM through the monitor's hostfwd_add and hostfwd_remove (forward_live.go).
// Windows watches the settings file for this; Linux applies the change when
// Settings saves, in the launcher that owns the VM, so a forward QEMU refuses
// is reported in Settings. A forward to guest port 22 still waits for the
// next launch, because the guest starts sshd at boot.

// linuxLiveForwards is set when this launch took its forwards from Settings.
// Command-line -forward and -ssh replace the saved list for one launch, and
// saving Settings must not rewrite those.
var linuxLiveForwards atomic.Bool

// startLinuxLiveForwards records the forwards QEMU starts with, which later
// Settings saves compare against.
func startLinuxLiveForwards(launched []portForward, fromSettings bool) {
	setLiveForwards(launched)
	linuxLiveForwards.Store(fromSettings)
}

// linuxForwardChange is what one Settings save did to the running VM.
type linuxForwardChange struct {
	changed  bool
	deferred []portForward
}

// planLinuxLiveForwards compares the running VM's forwards with a saved list.
func planLinuxLiveForwards(saved []string) (forwardPlan, error) {
	var desired forwardList
	for _, value := range saved {
		if err := desired.Set(value); err != nil {
			return forwardPlan{}, err
		}
	}
	return planLiveForwards(forwardsForBoot(nil), desired), nil
}

// applyLinuxLiveForwards brings the running VM's forwards to the saved list.
// What changed stays recorded even when another forward fails, so saving
// again retries only the rest.
func applyLinuxLiveForwards(parent context.Context, saved []string) (linuxForwardChange, error) {
	plan, err := planLinuxLiveForwards(saved)
	if err != nil {
		return linuxForwardChange{}, err
	}
	change := linuxForwardChange{deferred: plan.deferred}
	if len(plan.add)+len(plan.remove) == 0 {
		return change, nil
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	qmp, err := dialQMPControl(ctx, qmpToolsPort)
	if err != nil {
		return change, err
	}
	defer qmp.Close()
	monitor := func(line string) (string, error) {
		var reply string
		err := qmp.Call(ctx, "human-monitor-command", map[string]any{"command-line": line}, &reply)
		return reply, err
	}
	active, errs := applyLiveForwards(forwardsForBoot(nil), plan, monitor)
	setLiveForwards(active)
	change.changed = len(errs) < len(plan.add)+len(plan.remove)
	for _, err := range errs {
		logf("forwards: %v", err)
	}
	list := forwardList(active)
	logf("forwards: changed while running; now %q", list.String())
	return change, errors.Join(errs...)
}
