//go:build linux

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const linuxLoginName = "org.freedesktop.login1"
const linuxLoginManager = linuxLoginName + ".Manager"
const linuxLoginPath = dbus.ObjectPath("/org/freedesktop/login1")

// The source owns a private bus connection, never the tray or portal bus.
type linuxSleepSource interface {
	signals() <-chan *dbus.Signal
	done() <-chan struct{}
	inhibit(context.Context) (io.Closer, error)
	close()
}

type logindSleepSource struct {
	conn   *dbus.Conn
	events chan *dbus.Signal
}

func connectLogindSleep(ctx context.Context) (*logindSleepSource, error) {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	source := &logindSleepSource{conn: conn, events: make(chan *dbus.Signal, 16)}
	conn.Signal(source.events)
	budget, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err = conn.AddMatchSignalContext(budget,
		dbus.WithMatchSender(linuxLoginName), dbus.WithMatchObjectPath(linuxLoginPath),
		dbus.WithMatchInterface(linuxLoginManager), dbus.WithMatchMember("PrepareForSleep"))
	if err != nil {
		source.close()
		return nil, err
	}
	return source, nil
}

func (s *logindSleepSource) signals() <-chan *dbus.Signal { return s.events }
func (s *logindSleepSource) done() <-chan struct{}        { return s.conn.Context().Done() }
func (s *logindSleepSource) close() {
	s.conn.RemoveSignal(s.events)
	s.conn.Close()
}
func (s *logindSleepSource) inhibit(ctx context.Context) (io.Closer, error) {
	var fd dbus.UnixFD
	err := s.conn.Object(linuxLoginName, linuxLoginPath).CallWithContext(ctx,
		linuxLoginManager+".Inhibit", 0, "sleep", "Try Omarchy", uiText("shutdown.linux.sleep_reason"), "delay").Store(&fd)
	if err != nil {
		return nil, err
	}
	// godbus transfers ownership of the received descriptor to the caller.
	return os.NewFile(uintptr(fd), "logind-sleep-inhibitor"), nil
}

type linuxPowerMonitor struct {
	guest         *guestPowerState
	source        linuxSleepSource
	lock          io.Closer
	resumed       func()
	loggedFailure bool
}

func (p *linuxPowerMonitor) acquire(ctx context.Context) error {
	if p.lock != nil {
		return nil
	}
	budget, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	lock, err := p.source.inhibit(budget)
	if err == nil {
		p.lock = lock
	}
	return err
}

func (p *linuxPowerMonitor) release() {
	if p.lock != nil {
		p.lock.Close()
		p.lock = nil
	}
}

func (p *linuxPowerMonitor) close() {
	p.release()
	p.guest.close()
	p.source.close()
}

func (p *linuxPowerMonitor) handle(ctx context.Context, signal *dbus.Signal) {
	if signal == nil || signal.Path != linuxLoginPath || signal.Name != linuxLoginManager+".PrepareForSleep" || len(signal.Body) != 1 {
		return
	}
	sleeping, ok := signal.Body[0].(bool)
	if !ok {
		return
	}
	firstWake := p.guest.prepareForSleep(sleeping)
	if sleeping {
		// Always release, including a failed/unconfirmed pause. This is a delay
		// lock, not a veto on sleep. QMP has its own two-second transition budget.
		p.release()
		return
	}
	if firstWake {
		logf("linux resumed from sleep")
		p.resumed()
		if err := p.acquire(ctx); err != nil && !p.loggedFailure {
			p.loggedFailure = true
			logf("power: cannot delay host sleep: %v; continuing without a sleep delay", err)
		}
	}
}

// Start once controls answer, before waiting for the desktop. Stop before a
// replacement VM can start; the event loop is the sole owner of its QMP state.
func startLinuxPower(dir string, vmDone <-chan struct{}) func() {
	ctx, cancel := context.WithCancel(context.Background())
	source, err := connectLogindSleep(ctx)
	if err != nil {
		cancel()
		logf("power: host sleep handling unavailable: %v; continuing without it", err)
		return func() {}
	}
	p := &linuxPowerMonitor{
		source: source, resumed: notifyHostResumed,
		guest: &guestPowerState{ctx: ctx, identity: guestRuntimeGeneration.Load, changed: func(phase string) {
			setHostPowerTransition(phase)
			guestCompositorHealth.power(time.Now(), phase != "running")
		}, onRecovery: func(err error) {
			go createLinuxDiagnostics(dir)
			showLinuxRuntimeError(uiText("tray.power.paused_title"), uiText("tray.power.paused"))
		}, dial: func(ctx context.Context) (*qmpClient, error) {
			select {
			case <-vmDone:
				return nil, fmt.Errorf("guest runtime has exited")
			default:
				return dialQMPControl(ctx, qmpPowerRole)
			}
		}},
	}
	stop, err := p.start(ctx, vmDone)
	if err != nil {
		cancel()
		logf("power: host sleep handling unavailable: %v; continuing without it", err)
		return func() {}
	}
	return func() { stop(); cancel() }
}

func (p *linuxPowerMonitor) start(parent context.Context, vmDone <-chan struct{}) (func(), error) {
	ctx, cancel := context.WithCancel(parent)
	if err := p.acquire(ctx); err != nil {
		p.close()
		cancel()
		return nil, err
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer p.close()
		for {
			// Give shutdown priority over queued notifications.
			if ctx.Err() != nil {
				return
			}
			select {
			case <-vmDone:
				return
			default:
			}
			select {
			case <-ctx.Done():
				return
			case <-vmDone:
				return
			case <-p.source.done():
				logf("power: logind connection lost; continuing without host sleep handling")
				return
			case <-linuxResumeRequests:
				p.guest.manualResume()
			case signal, ok := <-p.source.signals():
				if !ok {
					return
				}
				p.handle(ctx, signal)
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { cancel(); <-finished }) }, nil
}
