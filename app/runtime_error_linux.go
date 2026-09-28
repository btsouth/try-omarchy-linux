//go:build linux

package main

import (
	"fmt"
	"sync/atomic"
)

var linuxRuntimeErrorVisible atomic.Bool

// Keep at most one independent error window open while the VM runs. Dismissing
// it must not cancel setup or shut down the guest.
func showLinuxRuntimeError(title, detail string) {
	if !linuxGUIEnabled || !linuxRuntimeErrorVisible.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer linuxRuntimeErrorVisible.Store(false)
		w := startLinuxWindow(func() {})
		if w == nil {
			return
		}
		showLinuxRuntimeErrorOnWindow(w, title, detail)
	}()
}

func showLinuxRuntimeErrorOnWindow(w *linuxSetupWindow, title, detail string) {
	w.expected.Store(true)
	w.update(linuxSetupState{Status: detail, Error: true, ErrorTitle: title})
	<-w.done
}

func reportLinuxFileDropError(err error) {
	if err == nil {
		return
	}
	logf("file drop: %v", err)
	showLinuxRuntimeError("File transfer failed", fmt.Sprintf("Omarchy could not receive the dropped files: %v\n\nCheck that Try Omarchy can access the files, then drop them again.", err))
}
