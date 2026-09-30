//go:build linux

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeLinuxMonitor answers on the private tools socket the way QEMU's human
// monitor answers hostfwd_add and hostfwd_remove.
type fakeLinuxMonitor struct {
	mu     sync.Mutex
	lines  []string
	refuse map[string]bool
}

func startFakeLinuxMonitor(t *testing.T) *fakeLinuxMonitor {
	t.Helper()
	// Unix sockets have a short path limit; the test name and TMPDIR may be long.
	control, err := os.MkdirTemp("/tmp", "forward-qmp-")
	if err != nil {
		t.Fatal(err)
	}
	previous := qmpControlDirectory
	qmpControlDirectory = func() (string, error) { return control, nil }
	listener, err := net.Listen("unix", filepath.Join(control, "tools.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		listener.Close()
		qmpControlDirectory = previous
		os.RemoveAll(control)
	})
	m := &fakeLinuxMonitor{refuse: map[string]bool{}}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go m.serve(conn)
		}
	}()
	return m
}

func (m *fakeLinuxMonitor) serve(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprintln(conn, `{"QMP":{"version":{"qemu":{"major":11}},"capabilities":[]}}`)
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var request struct {
			Execute   string `json:"execute"`
			ID        string `json:"id"`
			Arguments struct {
				CommandLine string `json:"command-line"`
			} `json:"arguments"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			return
		}
		switch request.Execute {
		case "qmp_capabilities":
			fmt.Fprintf(conn, "{\"return\":{},\"id\":%q}\n", request.ID)
		case "human-monitor-command":
			line := request.Arguments.CommandLine
			m.mu.Lock()
			m.lines = append(m.lines, line)
			refused := m.refuse[line]
			m.mu.Unlock()
			reply := ""
			if fields := strings.Fields(line); fields[0] == "hostfwd_remove" {
				reply = "host forwarding rule for " + fields[2] + " removed\r\n"
			} else if refused {
				reply = "could not set up host forwarding rule '" + fields[2] + "'\r\n"
			}
			fmt.Fprintf(conn, "{\"return\":%q,\"id\":%q}\n", reply, request.ID)
		default:
			// Live audio probes answer as if this QEMU had no audio routes.
			fmt.Fprintf(conn, "{\"error\":{\"class\":\"GenericError\",\"desc\":\"not in this test\"},\"id\":%q}\n", request.ID)
		}
	}
}

func (m *fakeLinuxMonitor) take() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	lines := m.lines
	m.lines = nil
	return lines
}

func resetLinuxLiveForwards(t *testing.T) {
	t.Cleanup(func() {
		liveForwardState.Lock()
		liveForwardState.active, liveForwardState.set = nil, false
		liveForwardState.Unlock()
		linuxLiveForwards.Store(false)
	})
}

func parsedForwards(t *testing.T, values ...string) []portForward {
	t.Helper()
	var list forwardList
	for _, value := range values {
		if err := list.Set(value); err != nil {
			t.Fatal(err)
		}
	}
	return list
}

func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func TestLinuxLiveForwardsChangeLocalForwardsAndRetryFailures(t *testing.T) {
	monitor := startFakeLinuxMonitor(t)
	resetLinuxLiveForwards(t)
	startLinuxLiveForwards(parsedForwards(t, "tcp:2222:22", "tcp:8080:80", "udp:5353:53"), true)
	monitor.refuse["hostfwd_add n0 tcp:127.0.0.1:9091-:91"] = true
	saved := []string{"tcp:2200:22", "tcp:127.0.0.1:8080:80", "tcp:9090:90", "tcp:9091:91"}
	ctx := context.Background()

	change, err := applyLinuxLiveForwards(ctx, saved)
	if err == nil || !strings.Contains(err.Error(), "could not set up host forwarding rule") {
		t.Fatalf("refused forward was not reported: %v", err)
	}
	if !change.changed || len(change.deferred) != 2 {
		t.Fatalf("change: %+v", change)
	}
	if notice := linuxForwardFailureNotice(change); !strings.Contains(notice, "some applied") {
		t.Fatalf("partial live update was hidden: %q", notice)
	}
	want := []string{"hostfwd_remove n0 udp:127.0.0.1:5353", "hostfwd_add n0 tcp:127.0.0.1:9090-:90", "hostfwd_add n0 tcp:127.0.0.1:9091-:91"}
	if got := monitor.take(); !reflect.DeepEqual(got, want) {
		t.Fatalf("monitor commands:\n got %q\nwant %q", got, want)
	}
	if got, want := forwardsForBoot(nil), parsedForwards(t, "tcp:2222:22", "tcp:8080:80", "tcp:9090:90"); !reflect.DeepEqual(got, want) {
		t.Fatalf("running forwards: %v, want %v", got, want)
	}

	// Saving again retries only what QEMU refused. The SSH forward keeps
	// waiting for the next launch, when the guest starts sshd.
	delete(monitor.refuse, "hostfwd_add n0 tcp:127.0.0.1:9091-:91")
	change, err = applyLinuxLiveForwards(ctx, saved)
	if err != nil || !change.changed || len(change.deferred) != 2 {
		t.Fatalf("retry: %+v %v", change, err)
	}
	if got := monitor.take(); !reflect.DeepEqual(got, []string{"hostfwd_add n0 tcp:127.0.0.1:9091-:91"}) {
		t.Fatalf("retry commands: %q", got)
	}
	change, err = applyLinuxLiveForwards(ctx, saved)
	if err != nil || change.changed || len(change.deferred) != 2 {
		t.Fatalf("unchanged save: %+v %v", change, err)
	}
	if got := monitor.take(); len(got) != 0 {
		t.Fatalf("unchanged save sent %q", got)
	}
	// A startup fallback relaunch builds its network from the running list.
	if got := forwardsForBoot(parsedForwards(t, "tcp:2222:22")); len(got) != 4 {
		t.Fatalf("relaunch forwards: %v", got)
	}
}

func TestLinuxLiveForwardGuestPortChangeKeepsItsHostPort(t *testing.T) {
	monitor := startFakeLinuxMonitor(t)
	resetLinuxLiveForwards(t)
	// The running VM's QEMU holds the forward's host port.
	held, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	port := held.Addr().(*net.TCPAddr).Port
	startLinuxLiveForwards(parsedForwards(t, fmt.Sprintf("tcp:%d:80", port)), true)
	saved := []string{fmt.Sprintf("tcp:%d:81", port)}
	plan, err := planLinuxLiveForwards(saved)
	if err != nil {
		t.Fatal(err)
	}
	if checkForwardBindings(plan.add) == nil {
		t.Fatal("the held port looked free")
	}
	if fresh := newForwardPorts(plan); len(fresh) != 0 {
		t.Fatalf("a guest port change asked for a new host port: %v", fresh)
	}
	if _, err := applyLinuxLiveForwards(context.Background(), saved); err != nil {
		t.Fatal(err)
	}
	want := []string{fmt.Sprintf("hostfwd_remove n0 tcp:127.0.0.1:%d", port), fmt.Sprintf("hostfwd_add n0 tcp:127.0.0.1:%d-:81", port)}
	if got := monitor.take(); !reflect.DeepEqual(got, want) {
		t.Fatalf("monitor commands:\n got %q\nwant %q", got, want)
	}
}

func TestLinuxLiveForwardSettingsHelper(t *testing.T) {
	dir := os.Getenv("TRY_OMARCHY_LIVE_FORWARD_DIR")
	if dir == "" {
		return
	}
	fail := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	fmt.Println(`{"event":"ready"}`)
	scanner := bufio.NewScanner(os.Stdin)
	count := 0
	for scanner.Scan() {
		var state linuxSetupState
		fail(json.Unmarshal(scanner.Bytes(), &state))
		if state.Prompt == "settings-saved" {
			fail(json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: "cancel"}))
			continue
		}
		if state.Prompt != "settings" {
			continue
		}
		if !state.Settings.ForwardsLive {
			panic("live forwards were not offered")
		}
		count++
		if count == 1 && !strings.Contains(state.Status, "Local port forwards apply when you save") {
			panic("Settings did not say forwards apply on save: " + state.Status)
		}
		form := *state.Settings
		switch count {
		case 1:
			form.Forwards = "tcp:" + os.Getenv("TRY_OMARCHY_LIVE_FORWARD_BUSY") + ":81"
		case 2:
			if state.Notice != "Check your settings before saving." || !strings.Contains(state.Status, "cannot open TCP") {
				panic("busy port was not reported: " + state.Status)
			}
			saved, err := loadSettings(settingsPath(dir))
			fail(err)
			if !strings.Contains(strings.Join(saved.Forwards, " "), ":80") {
				panic("a refused save changed the saved forwards")
			}
			form.Forwards = "tcp:" + os.Getenv("TRY_OMARCHY_LIVE_FORWARD_NEW") + ":80"
		default:
			// Another process can claim a released ephemeral port before
			// Settings validates it. Retry a fresh port, with its listener
			// closed so the test does not make its own validation fail.
			if count > 8 || !strings.Contains(state.Status, "cannot open TCP") {
				panic("forward save did not finish: " + state.Status)
			}
			form.Forwards = fmt.Sprintf("tcp:%d:80", freeLoopbackPort(t))
		}
		value, err := json.Marshal(form)
		fail(err)
		fail(json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: string(value)}))
	}
}

func TestLinuxRunningSettingsApplyPortForwards(t *testing.T) {
	monitor := startFakeLinuxMonitor(t)
	resetLinuxLiveForwards(t)
	busy, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	sshPort, oldPort, newPort := freeLoopbackPort(t), freeLoopbackPort(t), freeLoopbackPort(t)
	dir := t.TempDir()
	saved := []string{fmt.Sprintf("tcp:%d:80", oldPort), fmt.Sprintf("tcp:%d:22", sshPort)}
	if err := saveSettings(settingsPath(dir), settings{Forwards: saved}); err != nil {
		t.Fatal(err)
	}
	startLinuxLiveForwards(parsedForwards(t, saved...), true)
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxLiveForwardSettingsHelper$")
	cmd.Env = append(os.Environ(), "TRY_OMARCHY_LIVE_FORWARD_DIR="+dir,
		"TRY_OMARCHY_LIVE_FORWARD_BUSY="+strconv.Itoa(busy.Addr().(*net.TCPAddr).Port),
		"TRY_OMARCHY_LIVE_FORWARD_NEW="+strconv.Itoa(newPort))
	w := launchLinuxWindow(cmd, func() {})
	if w == nil {
		t.Fatal("settings helper did not start")
	}
	defer w.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	message := showLinuxSettingsInWindow(ctx, w, dir, true)
	if !strings.HasPrefix(message, "Port forwards applied.") || strings.Contains(message, "SSH changes") {
		t.Fatalf("saved message: %q", message)
	}
	after, err := loadSettings(settingsPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Forwards) != 2 {
		t.Fatalf("saved forwards: %q", after.Forwards)
	}
	applied, err := parseForward(after.Forwards[0])
	if err != nil || applied.guestPort != 80 || applied.hostPort == oldPort || applied.hostPort == busy.Addr().(*net.TCPAddr).Port {
		t.Fatalf("saved replacement: %+v %v", applied, err)
	}
	if after.Forwards[1] != fmt.Sprintf("tcp:%d:22", sshPort) {
		t.Fatalf("SSH forward changed: %q", after.Forwards)
	}
	want := []string{fmt.Sprintf("hostfwd_remove n0 tcp:127.0.0.1:%d", oldPort), fmt.Sprintf("hostfwd_add n0 tcp:127.0.0.1:%d-:80", applied.hostPort)}
	if got := monitor.take(); !reflect.DeepEqual(got, want) {
		t.Fatalf("monitor commands:\n got %q\nwant %q", got, want)
	}
}

func TestLinuxSettingsSavedMessageNamesWhatWaits(t *testing.T) {
	ssh := linuxForwardChange{deferred: []portForward{{proto: "tcp", hostPort: 2200, guestPort: 22}}}
	if got := linuxSettingsSavedMessage(false, ssh); !strings.HasPrefix(got, "Settings saved. SSH changes apply after shutting down Omarchy") {
		t.Fatalf("deferred SSH: %q", got)
	}
	if got := linuxSettingsSavedMessage(true, linuxForwardChange{changed: true}); !strings.HasPrefix(got, "Audio device choices applied. Port forwards applied. VM settings apply after") {
		t.Fatalf("audio and forwards: %q", got)
	}
}
