//go:build linux

package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLinuxDesktopWaitDoesNotUseQMPOrUserspaceAsDesktop(t *testing.T) {
	var desktop, userspace atomic.Bool
	userspace.Store(true)
	var stages []string
	lines := make(chan string, 2)
	lines <- `{"event":"DISPLAY_VISIBILITY","data":{"console":0,"visible":true}}`
	go func() {
		time.Sleep(80 * time.Millisecond)
		desktop.Store(true)
	}()
	result := waitLinuxDesktopReady(context.Background(), make(chan error), make(chan os.Signal), lines,
		&linuxVisibility{}, desktop.Load, userspace.Load, time.Second, 10*time.Millisecond,
		func(s string) { stages = append(stages, s) }, nil)
	if result != linuxDesktopReady || len(stages) != 1 || stages[0] != "Omarchy is starting its desktop..." {
		t.Fatalf("desktop handoff: result=%v stages=%v", result, stages)
	}
}

func TestLinuxShutdownAnswerSurvivesDesktopTransition(t *testing.T) {
	for _, transition := range []string{"ready", "timeout"} {
		for _, accept := range []bool{false, true} {
			t.Run(transition+map[bool]string{false: "-keep", true: "-shutdown"}[accept], func(t *testing.T) {
				var ready atomic.Bool
				lines := make(chan string, 2)
				lines <- `{"event":"DISPLAY_CLOSE_REQUEST"}`
				answer := make(chan bool, 1)
				prompted := make(chan struct{})
				prompts := 0
				confirmation := newLinuxShutdownConfirmation(func(context.Context) <-chan bool {
					prompts++
					close(prompted)
					return answer
				})
				defer confirmation.close()
				result := make(chan linuxDesktopWaitResult, 1)
				go func() {
					result <- waitLinuxDesktopReady(context.Background(), make(chan error), make(chan os.Signal), lines,
						&linuxVisibility{}, ready.Load, func() bool { return false }, 35*time.Millisecond,
						time.Millisecond, func(string) {}, confirmation)
				}()
				select {
				case <-prompted:
				case <-time.After(time.Second):
					t.Fatal("close request did not open confirmation")
				}
				if transition == "ready" {
					ready.Store(true)
				}
				want := linuxDesktopReady
				if transition == "timeout" {
					want = linuxDesktopTimedOut
				}
				select {
				case got := <-result:
					if got != want {
						t.Fatalf("transition=%v, want %v", got, want)
					}
				case <-time.After(time.Second):
					t.Fatal("desktop transition stalled on dialog")
				}
				if confirmation.answer() != answer || prompts != 1 {
					t.Fatalf("handoff lost or duplicated dialog: pending=%t prompts=%d", confirmation.answer() == answer, prompts)
				}
				qmp, peer := testQMPStream(t)
				commands := make(chan string, 2)
				go func() {
					scanner := bufio.NewScanner(peer)
					for scanner.Scan() {
						commands <- scanner.Text()
					}
				}()
				exited := make(chan error, 1)
				watchDone := make(chan struct{})
				oldGUI := linuxGUIEnabled
				linuxGUIEnabled = false // no session service in this protocol test
				defer func() { linuxGUIEnabled = oldGUI }()
				go func() {
					watchLinux(&config{}, qmp, &exec.Cmd{}, exited, make(chan os.Signal), nil,
						&linuxVisibility{}, 0, confirmation, false)
					close(watchDone)
				}()
				answer <- accept
				if accept {
					select {
					case command := <-commands:
						if !strings.Contains(command, "system_powerdown") {
							t.Fatalf("command: %s", command)
						}
					case <-time.After(time.Second):
						t.Fatal("confirmed shutdown was lost")
					}
				} else {
					select {
					case command := <-commands:
						t.Fatalf("Keep running sent %s", command)
					case <-time.After(30 * time.Millisecond):
					}
				}
				exited <- nil
				select {
				case <-watchDone:
				case <-time.After(time.Second):
					t.Fatal("supervisor did not exit")
				}
				select {
				case command := <-commands:
					t.Fatalf("duplicate command: %s", command)
				default:
				}
			})
		}
	}
}

func TestLinuxShutdownPromptCancelledOnProcessExit(t *testing.T) {
	ctxClosed := make(chan struct{})
	confirmation := newLinuxShutdownConfirmation(func(ctx context.Context) <-chan bool {
		go func() { <-ctx.Done(); close(ctxClosed) }()
		return make(chan bool)
	})
	confirmation.request()
	qmp, _ := testQMPStream(t)
	exited := make(chan error, 1)
	exited <- nil
	watchLinux(&config{}, qmp, &exec.Cmd{}, exited, make(chan os.Signal), nil,
		&linuxVisibility{}, 0, confirmation, false)
	select {
	case <-ctxClosed:
	case <-time.After(time.Second):
		t.Fatal("dialog survived QEMU exit")
	}
}

func TestLinuxLateDesktopClosesTimeoutMessage(t *testing.T) {
	oldUI := linuxUI
	linuxUI = newLinuxProgressUI()
	defer func() { linuxUI = oldUI }()
	oldReady := desktopReady.Load()
	desktopReady.Store(true)
	defer desktopReady.Store(oldReady)
	linuxUI.window = setupWindowFixture(t, "normal")
	linuxUI.showDesktopTimeout("desktop still starting")
	qmp, _ := testQMPStream(t)
	exited := make(chan error, 1)
	exited <- nil
	watchLinux(&config{}, qmp, &exec.Cmd{}, exited, make(chan os.Signal), nil,
		&linuxVisibility{}, 0, newLinuxShutdownConfirmation(nil), true)
	if linuxUI.window != nil || setupCancelled() {
		t.Fatal("late desktop left an error visible or cancelled the VM")
	}
}

func TestLinuxBootCancelRetriesACPIAfterUserspaceReady(t *testing.T) {
	oldInterval := linuxShutdownRetryInterval
	linuxShutdownRetryInterval = 20 * time.Millisecond
	defer func() { linuxShutdownRetryInterval = oldInterval }()
	oldReady := guestReady.Load()
	guestReady.Store(false)
	defer guestReady.Store(oldReady)
	oldGUI := linuxGUIEnabled
	linuxGUIEnabled = false
	defer func() { linuxGUIEnabled = oldGUI }()
	qmp, peer := testQMPStream(t)
	commands := make(chan string, 4)
	go func() {
		scanner := bufio.NewScanner(peer)
		for scanner.Scan() {
			commands <- scanner.Text()
		}
	}()
	exited := make(chan error, 1)
	watchDone := make(chan struct{})
	confirmation := newLinuxShutdownConfirmation(nil)
	go func() {
		watchLinux(&config{}, qmp, &exec.Cmd{}, exited, make(chan os.Signal), nil,
			&linuxVisibility{}, 1, confirmation, false)
		close(watchDone)
	}()
	select {
	case command := <-commands:
		t.Fatalf("retried before guest userspace: %s", command)
	case <-time.After(45 * time.Millisecond):
	}
	guestReady.Store(true)
	select {
	case command := <-commands:
		if !strings.Contains(command, "system_powerdown") || strings.Contains(command, "quit") {
			t.Fatalf("retry was not ACPI: %s", command)
		}
	case <-time.After(time.Second):
		t.Fatal("early ACPI request was not retried after userspace")
	}
	exited <- nil
	select {
	case <-watchDone:
	case <-time.After(time.Second):
		t.Fatal("supervision did not finish after process exit")
	}
}

func TestLinuxDesktopWaitFailurePaths(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ctx    context.Context
		exited <-chan error
		stop   <-chan os.Signal
		want   linuxDesktopWaitResult
	}{
		{name: "timeout", ctx: context.Background(), exited: make(chan error), stop: make(chan os.Signal), want: linuxDesktopTimedOut},
		{name: "exit", ctx: context.Background(), exited: func() <-chan error { c := make(chan error, 1); c <- nil; return c }(), stop: make(chan os.Signal), want: linuxDesktopExited},
		{name: "cancel", ctx: func() context.Context { c, cancel := context.WithCancel(context.Background()); cancel(); return c }(), exited: make(chan error), stop: make(chan os.Signal), want: linuxDesktopCancelled},
		{name: "interrupt", ctx: context.Background(), exited: make(chan error), stop: func() <-chan os.Signal { c := make(chan os.Signal, 1); c <- os.Interrupt; return c }(), want: linuxDesktopInterrupted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := waitLinuxDesktopReady(tc.ctx, tc.exited, tc.stop, nil, &linuxVisibility{},
				func() bool { return false }, func() bool { return false }, 20*time.Millisecond,
				5*time.Millisecond, func(string) {}, nil)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLinuxDesktopWaitConfirmsEarlyWindowClose(t *testing.T) {
	for _, tc := range []struct {
		name   string
		accept bool
		want   linuxDesktopWaitResult
	}{
		{name: "keep running", accept: false, want: linuxDesktopTimedOut},
		{name: "shut down", accept: true, want: linuxDesktopInterrupted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := make(chan string, 1)
			lines <- `{"event":"DISPLAY_CLOSE_REQUEST"}`
			prompts := 0
			confirmation := newLinuxShutdownConfirmation(func(context.Context) <-chan bool {
				prompts++
				result := make(chan bool, 1)
				result <- tc.accept
				return result
			})
			defer confirmation.close()
			got := waitLinuxDesktopReady(context.Background(), make(chan error), make(chan os.Signal),
				lines, &linuxVisibility{}, func() bool { return false }, func() bool { return false },
				25*time.Millisecond, 5*time.Millisecond, func(string) {}, confirmation)
			if got != tc.want || prompts != 1 {
				t.Fatalf("got %v with %d prompts; want %v", got, prompts, tc.want)
			}
		})
	}
}
