//go:build linux

package main

import (
	"bufio"
	"net"
	"testing"
	"time"
)

func TestLinuxVisibilityRequiresFreshPrimaryWindowReport(t *testing.T) {
	now := time.Now()
	v := &linuxVisibility{}
	if v.command(now) != "host-window hidden" {
		t.Fatal("old runtime inhibited idle")
	}
	for _, line := range []string{`{"event":"DISPLAY_VISIBILITY","data":{"visible":true}}`, `{"event":"DISPLAY_VISIBILITY","data":{"console":1,"visible":true}}`, `{"event":"DISPLAY_VISIBILITY","data":{"console":0}}`, `{"event":"other","data":{"console":0,"visible":true}}`} {
		if v.receive(line, now) {
			t.Fatalf("accepted %s", line)
		}
	}
	if !v.receive(`{"event":"DISPLAY_VISIBILITY","data":{"console":0,"visible":true}}`, now) {
		t.Fatal("valid report ignored")
	}
	if v.command(now.Add(14*time.Second)) != "host-window visible" {
		t.Fatal("lease lost early")
	}
	if v.command(now.Add(15*time.Second)) != "host-window hidden" {
		t.Fatal("stale QMP keeps guest awake")
	}
	v.receive(`{"event":"DISPLAY_VISIBILITY","data":{"console":0,"visible":false}}`, now)
	if v.command(now) != "host-window hidden" {
		t.Fatal("minimized window keeps guest awake")
	}
}

func TestLinuxCloseEventRequiresEventField(t *testing.T) {
	if closeRequested(`{"error":{"desc":"event DISPLAY_CLOSE_REQUEST"}}`) || closeRequested(`{"event":"OTHER","data":"DISPLAY_CLOSE_REQUEST"}`) {
		t.Fatal("unrelated QMP data requested shutdown")
	}
	if !closeRequested(`{"event":"DISPLAY_CLOSE_REQUEST","data":{"console":0}}`) {
		t.Fatal("close ignored")
	}
}

func TestLinuxVisibilityReachesGuestAgent(t *testing.T) {
	host, guest := net.Pipe()
	defer host.Close()
	defer guest.Close()
	old := theAgent.Load()
	defer theAgent.Store(old)
	a := newGuestAgent()
	a.conn = host
	theAgent.Store(a)
	received := make(chan string, 1)
	go func() {
		guest.SetReadDeadline(time.Now().Add(time.Second))
		line, _ := bufio.NewReader(guest).ReadString('\n')
		received <- line
	}()
	sendLinuxVisibility(&linuxVisibility{visible: true, reported: time.Now()})
	if got := <-received; got != "host-window visible\n" {
		t.Fatalf("guest received %q", got)
	}
}
