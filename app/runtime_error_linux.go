//go:build linux

package main

import (
	"fmt"
	"os"
	"sync"
)

type linuxRuntimeErrorMessage struct{ title, detail string }

// Show one independent error window at a time while the VM runs. A later error
// waits for the current one instead of being lost, so a graphics warning still
// appears after an earlier clipboard error. A title already open or waiting
// keeps only its newest detail, so repeated failures cannot stack up windows.
// Dismissing a window must not cancel setup or shut down the guest.
var linuxRuntimeErrors struct {
	sync.Mutex
	active  bool
	showing string
	pending []linuxRuntimeErrorMessage
}

var showLinuxRuntimeErrorWindow = func(title, detail string) {
	w := startLinuxWindow(func() {})
	if w == nil {
		return
	}
	showLinuxRuntimeErrorOnWindow(w, title, detail)
}

func showLinuxRuntimeError(title, detail string) {
	if !linuxGUIEnabled {
		return
	}
	queueLinuxRuntimeError(title, detail)
}

func queueLinuxRuntimeError(title, detail string) {
	q := &linuxRuntimeErrors
	q.Lock()
	defer q.Unlock()
	if q.active && q.showing == title {
		return
	}
	for i := range q.pending {
		if q.pending[i].title == title {
			q.pending[i].detail = detail
			return
		}
	}
	q.pending = append(q.pending, linuxRuntimeErrorMessage{title, detail})
	if !q.active {
		q.active = true
		go drainLinuxRuntimeErrors()
	}
}

func drainLinuxRuntimeErrors() {
	q := &linuxRuntimeErrors
	for {
		q.Lock()
		if len(q.pending) == 0 {
			q.active, q.showing = false, ""
			q.Unlock()
			return
		}
		next := q.pending[0]
		q.pending = q.pending[1:]
		q.showing = next.title
		q.Unlock()
		showLinuxRuntimeErrorWindow(next.title, next.detail)
	}
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
	showLinuxRuntimeError(uiText("error.linux.file_transfer_failed"), uiTextWith("error.linux.omarchy_could_not_receive_the_dropped_files_check", map[string]string{"error": fmt.Sprintf("%v", err)}))
}

// The setup window has closed by desktop readiness. Show the failure in a
// fresh window and wait for it before the launcher exits with failure.
func reportLinuxQEMUFailure(err error) {
	logf("QEMU runtime failure: %v", err)
	fmt.Fprintf(os.Stderr, "%s: %v\n", appTitle, err)
	if linuxGUIEnabled {
		showLinuxRuntimeErrorWindow(uiText("error.linux.omarchy_stopped_unexpectedly"), err.Error())
	}
}
