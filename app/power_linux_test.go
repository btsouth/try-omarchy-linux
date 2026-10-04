//go:build linux

package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

type fakeSleepLock struct {
	closed      chan struct{}
	beforeClose func()
}

func (l *fakeSleepLock) Close() error {
	if l.beforeClose != nil {
		l.beforeClose()
	}
	close(l.closed)
	return nil
}

type fakeSleepSource struct {
	events        chan *dbus.Signal
	disconnected  chan struct{}
	closed        chan struct{}
	locks         chan *fakeSleepLock
	fail          bool
	beforeRelease func()
}

func newFakeSleepSource() *fakeSleepSource {
	return &fakeSleepSource{events: make(chan *dbus.Signal, 16), disconnected: make(chan struct{}), closed: make(chan struct{}), locks: make(chan *fakeSleepLock, 8)}
}
func (s *fakeSleepSource) signals() <-chan *dbus.Signal { return s.events }
func (s *fakeSleepSource) done() <-chan struct{}        { return s.disconnected }
func (s *fakeSleepSource) close()                       { close(s.closed) }
func (s *fakeSleepSource) inhibit(ctx context.Context) (io.Closer, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 2*time.Second {
		return nil, errors.New("missing command budget")
	}
	if s.fail {
		return nil, errors.New("inhibit denied")
	}
	lock := &fakeSleepLock{closed: make(chan struct{}), beforeClose: s.beforeRelease}
	s.locks <- lock
	return lock, nil
}
func sleepSignal(sleeping bool) *dbus.Signal {
	return &dbus.Signal{Path: linuxLoginPath, Name: linuxLoginManager + ".PrepareForSleep", Body: []any{sleeping}}
}
func waitPower(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("power transition timed out")
	}
}
func nextSleepLock(t *testing.T, ch <-chan *fakeSleepLock) *fakeSleepLock {
	t.Helper()
	select {
	case lock := <-ch:
		return lock
	case <-time.After(3 * time.Second):
		t.Fatal("inhibitor not acquired")
		return nil
	}
}

func TestLinuxPowerSignalLifecycle(t *testing.T) {
	peer := &powerPeer{state: "running"}
	source := newFakeSleepSource()
	resumed := make(chan struct{}, 8)
	p := &linuxPowerMonitor{guest: powerFixture(t, peer), source: source, resumed: func() { resumed <- struct{}{} }}
	vmDone := make(chan struct{})
	stop, err := p.start(context.Background(), vmDone)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	lock := nextSleepLock(t, source.locks)
	for i := 0; i < 3; i++ {
		source.events <- sleepSignal(true)
		source.events <- sleepSignal(true)
		waitPower(t, lock.closed)
		peer.assert(t, "paused", repeatPowerOps(i, true)...)
		source.events <- sleepSignal(false)
		source.events <- sleepSignal(false)
		waitPower(t, resumed)
		lock = nextSleepLock(t, source.locks)
		peer.assert(t, "running", repeatPowerOps(i+1, false)...)
	}
	close(vmDone)
	waitPower(t, source.closed)
	waitPower(t, lock.closed)
	stop()
	stop()
	select {
	case <-resumed:
		t.Fatal("duplicate wake corrected clock again")
	default:
	}
}

func TestLinuxPowerReleasesAfterPauseAndOnFailure(t *testing.T) {
	for _, fail := range []string{"", "stop", "query-status"} {
		t.Run(fail, func(t *testing.T) {
			peer := &powerPeer{state: "running", fail: fail}
			source := newFakeSleepSource()
			released := false
			source.beforeRelease = func() {
				peer.mu.Lock()
				defer peer.mu.Unlock()
				if fail == "" && peer.state != "paused" {
					t.Error("released inhibitor before pause")
				}
				if len(peer.commands) == 0 {
					t.Error("released before inspection")
				}
				released = true
			}
			p := &linuxPowerMonitor{guest: powerFixture(t, peer), source: source, resumed: func() {}}
			if err := p.acquire(context.Background()); err != nil {
				t.Fatal(err)
			}
			p.handle(context.Background(), sleepSignal(true))
			if !released || p.lock != nil {
				t.Fatal("sleep remained inhibited")
			}
			p.close()
		})
	}
}

func TestLinuxPowerMalformedSignalsAndDisconnect(t *testing.T) {
	peer := &powerPeer{state: "running"}
	source := newFakeSleepSource()
	p := &linuxPowerMonitor{guest: powerFixture(t, peer), source: source, resumed: func() { t.Error("unexpected wake") }}
	stop, err := p.start(context.Background(), make(chan struct{}))
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	lock := nextSleepLock(t, source.locks)
	for _, signal := range []*dbus.Signal{nil, {}, {Path: linuxLoginPath, Name: linuxLoginManager + ".PrepareForSleep", Body: []any{"true"}}, {Path: linuxLoginPath, Name: "other.PrepareForSleep", Body: []any{true}}} {
		source.events <- signal
	}
	close(source.disconnected)
	waitPower(t, source.closed)
	waitPower(t, lock.closed)
	peer.assert(t, "running")
}

func TestLinuxPowerInhibitorFailure(t *testing.T) {
	peer := &powerPeer{state: "running"}
	source := newFakeSleepSource()
	source.fail = true
	p := &linuxPowerMonitor{guest: powerFixture(t, peer), source: source}
	if _, err := p.start(context.Background(), make(chan struct{})); err == nil {
		t.Fatal("accepted missing inhibitor")
	}
	waitPower(t, source.closed)
	peer.assert(t, "running")
}

// A test-owned bus and fake login1 exercise matching, Unix FD passing and
// descriptor release. The real system bus and host power APIs are never used.
type testLoginInhibitor struct{ read, write *os.File }
type testLoginManager struct{ locks chan testLoginInhibitor }

func (m *testLoginManager) Inhibit(what, who, why, mode string) (dbus.UnixFD, *dbus.Error) {
	if what != "sleep" || mode != "delay" || who != "Try Omarchy" || why == "" {
		return 0, dbus.MakeFailedError(errors.New("incorrect inhibitor arguments"))
	}
	read, write, err := os.Pipe()
	if err != nil {
		return 0, dbus.MakeFailedError(err)
	}
	fd := dbus.UnixFD(write.Fd())
	m.locks <- testLoginInhibitor{read, write}
	return fd, nil
}

func privatePowerBus(t *testing.T) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon unavailable")
	}
	cmd := exec.Command(daemon, "--session", "--nofork", "--print-address=1", "--address=unix:path="+filepath.Join(t.TempDir(), "bus"))
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	address := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(out).ReadString('\n'); address <- line }()
	select {
	case line := <-address:
		if len(line) == 0 {
			t.Fatal("private bus did not start")
		}
		return line[:len(line)-1]
	case <-time.After(3 * time.Second):
		t.Fatal("private bus startup timed out")
		return ""
	}
}

func TestLinuxPowerPrivateLoginBus(t *testing.T) {
	address := privatePowerBus(t)
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", address)
	server, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if reply, err := server.RequestName(linuxLoginName, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("login1 name: %v %v", reply, err)
	}
	manager := &testLoginManager{locks: make(chan testLoginInhibitor, 8)}
	if err := server.Export(manager, linuxLoginPath, linuxLoginManager); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source, err := connectLogindSleep(ctx)
	if err != nil {
		t.Fatal(err)
	}
	peer := &powerPeer{state: "running"}
	resumed := make(chan struct{}, 2)
	p := &linuxPowerMonitor{guest: powerFixture(t, peer), source: source, resumed: func() { resumed <- struct{}{} }}
	if err := p.acquire(ctx); err != nil {
		p.close()
		t.Fatal(err)
	}
	defer p.close()
	receive := func() {
		t.Helper()
		timeout := time.NewTimer(3 * time.Second)
		defer timeout.Stop()
		for {
			select {
			case signal := <-source.signals():
				// A new bus connection can also receive its own NameAcquired signal.
				if signal != nil && signal.Path == linuxLoginPath && signal.Name == linuxLoginManager+".PrepareForSleep" {
					p.handle(ctx, signal)
					return
				}
			case <-timeout.C:
				t.Fatal("login1 signal was not delivered")
			}
		}
	}
	getLock := func() testLoginInhibitor {
		t.Helper()
		select {
		case lock := <-manager.locks:
			// acquire has consumed the reply, so close the sender's copy.
			lock.write.Close()
			t.Cleanup(func() { lock.read.Close(); lock.write.Close() })
			return lock
		case <-time.After(3 * time.Second):
			t.Fatal("login1 Inhibit was not called")
			return testLoginInhibitor{}
		}
	}
	eof := func(lock testLoginInhibitor) {
		t.Helper()
		lock.read.SetReadDeadline(time.Now().Add(3 * time.Second))
		var b [1]byte
		if _, err := lock.read.Read(b[:]); err != io.EOF {
			t.Fatalf("inhibitor FD not released: %v", err)
		}
	}
	first := getLock()
	if err := server.Emit(linuxLoginPath, linuxLoginManager+".PrepareForSleep", true); err != nil {
		t.Fatal(err)
	}
	receive()
	eof(first)
	peer.assert(t, "paused", "stop")
	if err := server.Emit(linuxLoginPath, linuxLoginManager+".PrepareForSleep", false); err != nil {
		t.Fatal(err)
	}
	receive()
	waitPower(t, resumed)
	second := getLock()
	p.close()
	eof(second)
	peer.assert(t, "running", "stop", "cont")
}

func TestLinuxPowerQEMURuntime(t *testing.T) {
	qemu := os.Getenv("QEMU_SYSTEM")
	if qemu == "" {
		t.Skip("set QEMU_SYSTEM for diskless QMP runtime evidence")
	}
	dir := t.TempDir()
	socket := filepath.Join(dir, "power.sock")
	toolsSocket := filepath.Join(dir, "tools.sock")
	cmd := exec.Command(qemu, "-machine", "none", "-nodefaults", "-display", "none", "-S", "-qmp", "unix:"+socket+",server=on,wait=off", "-qmp", "unix:"+toolsSocket+",server=on,wait=off")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	dial := func(ctx context.Context) (*qmpClient, error) {
		var conn net.Conn
		var err error
		for ctx.Err() == nil {
			conn, err = (&net.Dialer{}).DialContext(ctx, "unix", socket)
			if err == nil {
				return newQMPClient(ctx, conn)
			}
			time.Sleep(10 * time.Millisecond)
		}
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tools, err := func() (*qmpClient, error) {
		// Wait for socket creation before connecting the independent tools role.
		ready, err := dial(ctx)
		if err != nil {
			return nil, err
		}
		ready.Close()
		conn, err := (&net.Dialer{}).DialContext(ctx, "unix", toolsSocket)
		if err != nil {
			return nil, err
		}
		return newQMPClient(ctx, conn)
	}()
	if err != nil {
		t.Fatal(err)
	}
	defer tools.Close()
	p := &guestPowerState{dial: dial}
	defer p.close()
	status := func(command, want string) {
		t.Helper()
		budget, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if command != "" {
			if err := tools.Call(budget, command, nil, nil); err != nil {
				t.Fatal(err)
			}
		}
		var state vmRuntimeStatus
		if err := tools.Call(budget, "query-status", nil, &state); err != nil || state.Status != want {
			t.Fatalf("status=%+v err=%v want=%s", state, err, want)
		}
	}
	p.prepareForSleep(true)
	p.prepareForSleep(false)
	status("", "prelaunch")
	status("cont", "running")
	for i := 0; i < 3; i++ {
		p.prepareForSleep(true)
		p.prepareForSleep(true)
		if !p.owned {
			t.Fatal("runtime pause not owned")
		}
		status("", "paused")
		p.prepareForSleep(false)
		p.prepareForSleep(false)
		status("", "running")
	}
	p.prepareForSleep(true)
	status("cont", "running")
	status("stop", "paused")
	p.prepareForSleep(false)
	status("", "paused")
}
