package main

import (
	"errors"
	"sync/atomic"
)

// theAgent is the running launcher's guest agent channel; nil until it is
// listening.
var theAgent atomic.Pointer[guestAgent]

// requestReclaim asks the guest to zero its free space so disk.raw can be
// compacted after shutdown. Used by the tray, and by "-reclaim" through the
// lifecycle port.
// reclaimDir is the data directory whose host drive bounds a reclaim pass.
var reclaimDir atomic.Pointer[string]
var reclaimSupported atomic.Bool

// reclaimFreeBytes reads the host drive's free space; tests replace it.
var reclaimFreeBytes = diskFreeBytes

// requestReclaimError starts a pass with a budget from the host drive's free
// space, or says why it cannot start.
func requestReclaimError() error {
	if !reclaimSupported.Load() {
		return errors.New(reclaimUnsupportedMessage())
	}
	dir := reclaimDir.Load()
	a := theAgent.Load()
	if dir == nil || a == nil {
		return errors.New(uiText("reclaim.error.not_ready"))
	}
	free, err := reclaimFreeBytes(*dir)
	if err != nil {
		return uiError(uiTextWith("reclaim.error.free_space", map[string]string{"error": err.Error()}), err)
	}
	budget := reclaimBudgetMiB(free)
	if budget == 0 {
		return errors.New(reclaimNeedsSpaceMessage())
	}
	if !a.requestZeroFill(budget) {
		return errors.New(uiTextWith("reclaim.error.not_started", map[string]string{"status": a.reclaimStatus()}))
	}
	return nil
}
