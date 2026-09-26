package main

import (
	"fmt"
	"sync/atomic"
)

// theAgent is the running launcher's guest agent channel; nil until it is
// listening.
var theAgent atomic.Pointer[guestAgent]

// requestReclaim asks the guest to zero its free space so disk.raw can be
// compacted after shutdown. Used by the tray, and by "-reclaim" through the
// lifecycle port.
// reclaimDir is the data directory whose Windows drive bounds a reclaim pass.
var reclaimDir atomic.Pointer[string]
var reclaimSupported atomic.Bool

func requestReclaimError() error {
	if !reclaimSupported.Load() {
		return fmt.Errorf("Reclaim is available for standard raw disks only.")
	}
	dir := reclaimDir.Load()
	a := theAgent.Load()
	if dir == nil || a == nil {
		return fmt.Errorf("Omarchy is not ready. Wait for the desktop and try again.")
	}
	free, err := diskFreeBytes(*dir)
	if err != nil {
		return fmt.Errorf("Could not check free space: %w", err)
	}
	budget := reclaimBudgetMiB(free)
	if budget == 0 {
		return fmt.Errorf("Reclaim needs at least 4.25 GiB free on the Windows drive.")
	}
	if !a.requestZeroFill(budget) {
		return fmt.Errorf("Reclaim was not started. %s", a.reclaimStatus())
	}
	return nil
}
