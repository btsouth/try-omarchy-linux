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
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLinuxAudioRoutesRestorePlaybackAfterCaptureFailureAndRetry(t *testing.T) {
	type change struct{ Property, Value string }
	changes := make(chan []change, 1)
	qmp := qmpTestPeer(t, func(conn net.Conn, r *bufio.Reader) {
		routes := map[string]string{linuxAudioOutputProperty: "sink.old", linuxAudioInputProperty: "source.old"}
		var applied []change
		rejectInput := true
		for i := 0; i < 9; i++ {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			var request struct {
				Execute   string                                 `json:"execute"`
				ID        string                                 `json:"id"`
				Arguments struct{ Path, Property, Value string } `json:"arguments"`
			}
			if err := json.Unmarshal(line, &request); err != nil {
				return
			}
			if request.Arguments.Path != linuxAudioObject {
				return
			}
			if request.Execute == "qom-get" {
				fmt.Fprintf(conn, "{\"return\":%q,\"id\":%q}\n", routes[request.Arguments.Property], request.ID)
				continue
			}
			applied = append(applied, change{request.Arguments.Property, request.Arguments.Value})
			if request.Arguments.Property == linuxAudioInputProperty && rejectInput {
				rejectInput = false
				fmt.Fprintf(conn, "{\"error\":{\"class\":\"GenericError\",\"desc\":\"capture unavailable\"},\"id\":%q}\n", request.ID)
			} else {
				routes[request.Arguments.Property] = request.Arguments.Value
				fmt.Fprintf(conn, "{\"return\":{},\"id\":%q}\n", request.ID)
			}
		}
		changes <- applied
	})
	next := audioPreferences{SchemaVersion: 1, Output: "sink.new", Input: "source.new"}
	if err := applyLinuxAudioRoutesWithClient(context.Background(), qmp, next); err == nil || !strings.Contains(err.Error(), "capture unavailable") {
		t.Fatalf("capture failure was not reported: %v", err)
	}
	if err := applyLinuxAudioRoutesWithClient(context.Background(), qmp, next); err != nil {
		t.Fatal(err)
	}
	want := []change{
		{linuxAudioOutputProperty, "sink.new"},
		{linuxAudioInputProperty, "source.new"},
		{linuxAudioOutputProperty, "sink.old"},
		{linuxAudioOutputProperty, "sink.new"},
		{linuxAudioInputProperty, "source.new"},
	}
	if got := <-changes; !reflect.DeepEqual(got, want) {
		t.Fatalf("route changes: %+v", got)
	}
}

func TestLinuxAudioRoutesDoNotChangePlaybackWhenRuntimeLacksCaptureControl(t *testing.T) {
	qmp := qmpTestPeer(t, func(conn net.Conn, r *bufio.Reader) {
		id, err := qmpReadRequest(r)
		if err != nil {
			return
		}
		fmt.Fprintf(conn, "{\"return\":\"sink.old\",\"id\":%q}\n", id)
		id, err = qmpReadRequest(r)
		if err != nil {
			return
		}
		fmt.Fprintf(conn, "{\"error\":{\"class\":\"GenericError\",\"desc\":\"property missing\"},\"id\":%q}\n", id)
	})
	next := audioPreferences{SchemaVersion: 1, Output: "sink.new", Input: "source.new"}
	if err := applyLinuxAudioRoutesWithClient(context.Background(), qmp, next); err == nil || !strings.Contains(err.Error(), "property missing") {
		t.Fatalf("unsupported runtime was accepted: %v", err)
	}
}

func TestLinuxLiveAudioSaveRetryHelper(t *testing.T) {
	dir := os.Getenv("TRY_OMARCHY_LIVE_AUDIO_RETRY_DIR")
	if dir == "" {
		return
	}
	path := filepath.Join(dir, audioPreferencesFilename)
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
		if !state.Settings.AudioLive {
			panic("live audio was not detected")
		}
		count++
		switch count {
		case 1:
			fail(os.Rename(path, path+".backup"))
			fail(os.Mkdir(path, 0700))
		case 2:
			if state.Notice != "Could not save audio devices." || !strings.Contains(state.Status, "Could not save audio devices") || !strings.Contains(state.Status, "Audio devices have not been switched") {
				panic("save failure was hidden")
			}
			fail(os.Remove(path))
			fail(os.Rename(path+".backup", path))
		case 3:
			if state.Notice != "Audio choices saved; live switch failed." || !strings.Contains(state.Status, "Audio choices saved, but could not switch devices") {
				panic("live apply failure was hidden")
			}
		default:
			panic("audio retry did not finish")
		}
		form := *state.Settings
		form.AudioOutput, form.AudioInput = "sink.new", "source.new"
		value, err := json.Marshal(form)
		fail(err)
		fail(json.NewEncoder(os.Stdout).Encode(linuxSetupReply{Event: "reply", Request: state.Request, Value: string(value)}))
	}
}

func TestLinuxLiveAudioSettingsRetryAfterDiskAndRuntimeFailures(t *testing.T) {
	dir := t.TempDir()
	if err := saveAudioPreferences(dir, audioPreferences{SchemaVersion: 1, Output: "sink.old", Input: "source.old"}); err != nil {
		t.Fatal(err)
	}
	// Unix sockets have a short path limit; the test name and TMPDIR may be long.
	control, err := os.MkdirTemp("/tmp", "audio-qmp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(control) })
	previous := qmpControlDirectory
	qmpControlDirectory = func() (string, error) { return control, nil }
	defer func() { qmpControlDirectory = previous }()
	listener, err := net.Listen("unix", filepath.Join(control, "tools.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var mu sync.Mutex
	routes := map[string]string{linuxAudioOutputProperty: "sink.old", linuxAudioInputProperty: "source.old"}
	rejectInput := true
	failures := make(chan error, 1)
	report := func(err error) {
		select {
		case failures <- err:
		default:
		}
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(10 * time.Second))
				fmt.Fprintln(conn, `{"QMP":{"version":{"qemu":{"major":11}},"capabilities":[]}}`)
				scanner := bufio.NewScanner(conn)
				for scanner.Scan() {
					var request struct {
						Execute, ID string
						Arguments   struct{ Path, Property, Value string }
					}
					if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
						report(err)
						return
					}
					if request.Execute == "qmp_capabilities" {
						fmt.Fprintf(conn, "{\"return\":{},\"id\":%q}\n", request.ID)
						continue
					}
					mu.Lock()
					if request.Execute == "qom-get" {
						value := routes[request.Arguments.Property]
						mu.Unlock()
						fmt.Fprintf(conn, "{\"return\":%q,\"id\":%q}\n", value, request.ID)
						continue
					}
					saved, err := loadAudioPreferences(dir)
					if err != nil || saved.Output != "sink.new" || saved.Input != "source.new" {
						report(fmt.Errorf("runtime changed before preferences were saved: %+v %v", saved, err))
					}
					if request.Arguments.Property == linuxAudioInputProperty && rejectInput {
						rejectInput = false
						mu.Unlock()
						fmt.Fprintf(conn, "{\"error\":{\"class\":\"GenericError\",\"desc\":\"capture unavailable\"},\"id\":%q}\n", request.ID)
						continue
					}
					routes[request.Arguments.Property] = request.Arguments.Value
					mu.Unlock()
					fmt.Fprintf(conn, "{\"return\":{},\"id\":%q}\n", request.ID)
				}
			}()
		}
	}()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxLiveAudioSaveRetryHelper$")
	cmd.Env = append(os.Environ(), "TRY_OMARCHY_LIVE_AUDIO_RETRY_DIR="+dir)
	w := launchLinuxWindow(cmd, func() {})
	if w == nil {
		t.Fatal("settings helper did not start")
	}
	defer w.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if got := showLinuxSettingsInWindow(ctx, w, dir, true); !strings.Contains(got, "Audio device choices applied") {
		t.Fatalf("retry failed: %q", got)
	}
	select {
	case err := <-failures:
		t.Fatal(err)
	default:
	}
	mu.Lock()
	defer mu.Unlock()
	if routes[linuxAudioOutputProperty] != "sink.new" || routes[linuxAudioInputProperty] != "source.new" {
		t.Fatalf("live routes did not follow retry: %+v", routes)
	}
}
