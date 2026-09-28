//go:build linux

package main

import (
	"testing"
	"time"
)

func TestLinuxRuntimeErrorsQueueInsteadOfDropping(t *testing.T) {
	previousGUI, previousWindow := linuxGUIEnabled, showLinuxRuntimeErrorWindow
	shown := make(chan linuxRuntimeErrorMessage, 8)
	dismiss := make(chan struct{})
	linuxGUIEnabled = true
	showLinuxRuntimeErrorWindow = func(title, detail string) {
		shown <- linuxRuntimeErrorMessage{title, detail}
		<-dismiss
	}
	t.Cleanup(func() { linuxGUIEnabled, showLinuxRuntimeErrorWindow = previousGUI, previousWindow })
	next := func() linuxRuntimeErrorMessage {
		t.Helper()
		select {
		case message := <-shown:
			return message
		case <-time.After(3 * time.Second):
			t.Fatal("queued error was never shown")
			return linuxRuntimeErrorMessage{}
		}
	}

	showLinuxRuntimeError("Clipboard access", "Clipboard declined.")
	if got := next(); got.title != "Clipboard access" {
		t.Fatalf("first window: %+v", got)
	}
	showLinuxRuntimeError("Clipboard access", "Clipboard declined again.")
	showLinuxRuntimeError("File transfer failed", "first drop")
	showLinuxRuntimeError("Graphics error", "Select Software rendering.")
	showLinuxRuntimeError("File transfer failed", "second drop")
	select {
	case message := <-shown:
		t.Fatalf("second window opened over the first: %+v", message)
	case <-time.After(100 * time.Millisecond):
	}

	dismiss <- struct{}{}
	if got := next(); got.title != "File transfer failed" || got.detail != "second drop" {
		t.Fatalf("waiting error lost its newest detail: %+v", got)
	}
	dismiss <- struct{}{}
	if got := next(); got.title != "Graphics error" {
		t.Fatalf("graphics warning was dropped: %+v", got)
	}
	dismiss <- struct{}{}
	select {
	case message := <-shown:
		t.Fatalf("duplicate of the dismissed error reopened: %+v", message)
	case <-time.After(100 * time.Millisecond):
	}

	showLinuxRuntimeError("Graphics error", "again after the queue drained")
	if got := next(); got.detail != "again after the queue drained" {
		t.Fatalf("queue did not restart: %+v", got)
	}
	dismiss <- struct{}{}
}
