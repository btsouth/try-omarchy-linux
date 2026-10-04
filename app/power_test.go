package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"testing"
)

type powerPeer struct {
	mu       sync.Mutex
	state    string
	commands []string
	fail     string
	lost     bool
	events   []string
}

func (s *powerPeer) client(t *testing.T) *qmpClient {
	return qmpTestPeer(t, func(conn net.Conn, reader *bufio.Reader) {
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				return
			}
			var request struct{ Execute, ID string }
			if json.Unmarshal(line, &request) != nil {
				return
			}
			s.mu.Lock()
			s.commands = append(s.commands, request.Execute)
			events := s.events
			s.events = nil
			fail := s.fail == request.Execute
			lost := fail && s.lost
			if lost && request.Execute == "stop" {
				s.state = "paused"
			}
			if !fail {
				if request.Execute == "stop" {
					s.state = "paused"
					events = append(events, "STOP")
				}
				if request.Execute == "cont" {
					s.state = "running"
					events = append(events, "RESUME")
				}
			}
			state := s.state
			s.mu.Unlock()
			if lost {
				return
			}
			for _, event := range events {
				fmt.Fprintf(conn, "{\"event\":%q}\n", event)
			}
			if fail {
				fmt.Fprintf(conn, "{\"error\":{\"class\":\"GenericError\",\"desc\":\"injected failure\"},\"id\":%q}\n", request.ID)
			} else if request.Execute == "query-status" {
				fmt.Fprintf(conn, "{\"return\":{\"running\":%t,\"status\":%q},\"id\":%q}\n", state == "running", state, request.ID)
			} else {
				fmt.Fprintf(conn, "{\"return\":{},\"id\":%q}\n", request.ID)
			}
		}
	})
}

func (s *powerPeer) assert(t *testing.T, state string, operations ...string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var actual []string
	for _, command := range s.commands {
		if command != "query-status" {
			actual = append(actual, command)
		}
	}
	if s.state != state || !reflect.DeepEqual(actual, operations) {
		t.Fatalf("state=%s operations=%v; want %s %v", s.state, actual, state, operations)
	}
}

func powerFixture(t *testing.T, peer *powerPeer) *guestPowerState {
	p := &guestPowerState{dial: func(context.Context) (*qmpClient, error) { return peer.client(t), nil }}
	t.Cleanup(p.close)
	return p
}

func TestPowerRepeatedTransitions(t *testing.T) {
	peer := &powerPeer{state: "running"}
	p := powerFixture(t, peer)
	p.prepareForSleep(false)
	p.prepareForSleep(false)
	peer.assert(t, "running")
	for i := 0; i < 3; i++ {
		p.prepareForSleep(true)
		p.prepareForSleep(true)
		peer.assert(t, "paused", repeatPowerOps(i, true)...)
		p.prepareForSleep(false)
		p.prepareForSleep(false)
		peer.assert(t, "running", repeatPowerOps(i+1, false)...)
	}
}

func repeatPowerOps(cycles int, paused bool) []string {
	var ops []string
	for i := 0; i < cycles; i++ {
		ops = append(ops, "stop", "cont")
	}
	if paused {
		ops = append(ops, "stop")
	}
	return ops
}

func TestPowerPreservesNonRunningGuests(t *testing.T) {
	for _, state := range []string{"paused", "prelaunch", "inmigrate", "shutdown", "suspended", "internal-error"} {
		t.Run(state, func(t *testing.T) {
			peer := &powerPeer{state: state}
			p := powerFixture(t, peer)
			p.prepareForSleep(true)
			p.prepareForSleep(false)
			p.prepareForSleep(false)
			peer.assert(t, state)
		})
	}
}

func TestPowerManualChangesWhileAsleep(t *testing.T) {
	for _, state := range []string{"running", "paused"} {
		t.Run(state, func(t *testing.T) {
			peer := &powerPeer{state: "running"}
			p := powerFixture(t, peer)
			p.prepareForSleep(true)
			peer.mu.Lock()
			peer.state = state
			peer.events = []string{"RESUME"}
			if state == "paused" {
				peer.events = append(peer.events, "STOP")
			}
			peer.mu.Unlock()
			p.prepareForSleep(false)
			p.prepareForSleep(false)
			peer.assert(t, state, "stop")
		})
	}
}

func TestPowerCommandFailures(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		lost          bool
	}{
		{"query rejected", "query-status", false}, {"stop rejected", "stop", false},
		{"query disconnected", "query-status", true}, {"lost stop reply", "stop", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peer := &powerPeer{state: "running", fail: tc.command, lost: tc.lost}
			p := powerFixture(t, peer)
			p.prepareForSleep(true)
			p.prepareForSleep(false)
			p.prepareForSleep(false)
			state := "running"
			if tc.command == "stop" && tc.lost {
				state = "paused"
			}
			if tc.command == "stop" {
				peer.assert(t, state, "stop")
			} else {
				peer.assert(t, state)
			}
			if p.client != nil || p.owned {
				t.Fatal("retained failed ownership")
			}
		})
	}
}

func TestPowerResumeFailureAndRetry(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint(lost), func(t *testing.T) {
			peer := &powerPeer{state: "running"}
			p := powerFixture(t, peer)
			p.prepareForSleep(true)
			peer.mu.Lock()
			peer.fail = "cont"
			peer.lost = lost
			peer.mu.Unlock()
			p.prepareForSleep(false)
			peer.assert(t, "paused", "stop", "cont")
			peer.mu.Lock()
			peer.fail = ""
			peer.mu.Unlock()
			p.prepareForSleep(false)
			if lost {
				peer.assert(t, "paused", "stop", "cont")
			} else {
				peer.assert(t, "running", "stop", "cont", "cont")
			}
		})
	}
}

func TestPowerRuntimeExitAndReplacement(t *testing.T) {
	peer := &powerPeer{state: "running"}
	p := powerFixture(t, peer)
	p.prepareForSleep(true)
	p.client.Close()
	replacement := &powerPeer{state: "paused"}
	p.dial = func(context.Context) (*qmpClient, error) { return replacement.client(t), nil }
	p.prepareForSleep(false)
	p.prepareForSleep(false)
	replacement.assert(t, "paused")
	p.prepareForSleep(true)
	p.prepareForSleep(false)
	replacement.assert(t, "paused")
}

func TestPowerUnavailableControls(t *testing.T) {
	p := &guestPowerState{dial: func(context.Context) (*qmpClient, error) { return nil, errors.New("unavailable") }}
	p.prepareForSleep(true)
	p.prepareForSleep(false)
	if p.owned || p.client != nil {
		t.Fatal("claimed unavailable guest")
	}
}
