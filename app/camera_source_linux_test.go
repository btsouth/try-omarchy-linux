//go:build linux

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestLinuxCameraCaptureHelper(t *testing.T) {
	mode := os.Getenv("TRYOMARCHY_CAMERA_HELPER")
	if mode == "" {
		return
	}
	if path := os.Getenv("TRYOMARCHY_CAMERA_HELPER_PID"); path != "" {
		if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			panic(err)
		}
	}
	if mode == "fail" {
		fmt.Fprintln(os.Stderr, "Camera device unavailable")
		os.Exit(1)
	}
	if mode == "stall" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	if mode == "frames-stall" {
		os.Stdout.Write(bytes.Repeat([]byte{42}, cameraFrameBytes))
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	for {
		if _, err := os.Stdout.Write(bytes.Repeat([]byte{42}, cameraFrameBytes)); err != nil {
			os.Exit(0)
		}
		time.Sleep(time.Second / 30)
	}
}

func TestLinuxCameraParentExitHelper(t *testing.T) {
	if os.Getenv("TRYOMARCHY_CAMERA_PARENT_EXIT") != "1" {
		return
	}
	s := &linuxCameraSource{helper: os.Getenv("TRYOMARCHY_CAMERA_HELPER_PATH"), remote: func(context.Context) (*os.File, error) { return os.Open(os.DevNull) }}
	if _, err := s.start(); err != nil {
		panic(err)
	}
	os.Exit(0) // Deliberately skip stop, as a crashed launcher would.
}

func TestLinuxCameraHelperDiesWithLauncher(t *testing.T) {
	s := testLinuxCameraSource(t, "frames-stall")
	pidFile := filepath.Join(t.TempDir(), "capture.pid")
	cmd := exec.Command(os.Args[0], "-test.run=^TestLinuxCameraParentExitHelper$")
	cmd.Env = append(os.Environ(), "TRYOMARCHY_CAMERA_PARENT_EXIT=1", "TRYOMARCHY_CAMERA_HELPER_PATH="+s.helper, "TRYOMARCHY_CAMERA_HELPER_PID="+pidFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("parent exit: %v %s", err, output)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if os.IsNotExist(err) {
			break
		}
		// An orphan may wait briefly for its namespace's init to reap it.
		if tail := strings.LastIndex(string(data), ") "); tail >= 0 && strings.HasPrefix(string(data[tail+2:]), "Z ") {
			break
		}
		if time.Now().After(deadline) {
			syscall.Kill(pid, syscall.SIGKILL)
			t.Fatal("camera helper survived launcher exit")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLinuxCameraConnectionIsPrivate(t *testing.T) {
	previous := qmpControlDirectory
	dir := filepath.Join(t.TempDir(), "control")
	qmpControlDirectory = func() (string, error) { return dir, nil }
	defer func() { qmpControlDirectory = previous }()
	listener, err := listenLinuxCamera()
	if err != nil {
		t.Fatal(err)
	}
	path, _ := linuxCameraSocketPath()
	for path, mode := range map[string]os.FileMode{dir: 0700, path: 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("private path permissions: %s %v", path, err)
		}
	}
	if second, err := listenLinuxCamera(); err == nil {
		second.Close()
		t.Fatal("camera connection replaced while active")
	}
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	listener.Close()
	stale, err := listenLinuxCamera()
	if err != nil {
		t.Fatalf("stale camera connection: %v", err)
	}
	stale.Close()
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if second, err := listenLinuxCamera(); err == nil {
		second.Close()
		t.Fatal("camera replaced an unrelated file")
	}
	if data, _ := os.ReadFile(path); string(data) != "keep" {
		t.Fatal("camera overwrote an unrelated file")
	}
}

func testLinuxCameraSource(t *testing.T, mode string) *linuxCameraSource {
	t.Helper()
	t.Setenv("TRYOMARCHY_CAMERA_HELPER", mode)
	helper := filepath.Join(t.TempDir(), "camera-helper")
	// The binary path is shell quoted; the chosen camera ID is not shell input.
	quoted := "'" + strings.ReplaceAll(os.Args[0], "'", "'\"'\"'") + "'"
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexec "+quoted+" -test.run=^TestLinuxCameraCaptureHelper$\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return &linuxCameraSource{helper: helper, remote: func(ctx context.Context) (*os.File, error) { return os.Open(os.DevNull) }}
}

func TestLinuxCameraCaptureStopsAndRestarts(t *testing.T) {
	s := testLinuxCameraSource(t, "frames")
	for i := 0; i < 2; i++ {
		frames, err := s.start()
		if err != nil {
			t.Fatal(err)
		}
		if frame := <-frames; len(frame) != cameraFrameBytes || frame[0] != 42 {
			t.Fatal("invalid capture frame")
		}
		s.stop()
		for range frames {
		} // EOF after stop, no helper or reader left running
	}
}

func TestLinuxCameraCaptureFailureAndCancellation(t *testing.T) {
	s := testLinuxCameraSource(t, "fail")
	if _, err := s.start(); err == nil || !strings.Contains(err.Error(), "Camera device unavailable") {
		t.Fatalf("failure: %v", err)
	}
	t.Setenv("TRYOMARCHY_CAMERA_HELPER", "stall")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := s.startContext(ctx); err == nil {
		t.Fatal("capture ignored cancellation")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("capture cancellation did not stop the helper")
	}
}

func TestLinuxCameraDisabledDoesNotRequestPermission(t *testing.T) {
	s := configuredCameraSource(desktopPreferences{CameraDisabled: true})
	if _, err := s.start(); err == nil || !strings.Contains(err.Error(), "Camera access is off") {
		t.Fatalf("disabled: %v", err)
	}
	if _, ok := s.(*linuxCameraSource); ok {
		t.Fatal("disabled camera can request access")
	}
}

type pendingCameraSource struct {
	requested chan struct{}
	cancelled chan struct{}
}

func (p *pendingCameraSource) start() (<-chan []byte, error) { panic("uncancellable start") }
func (p *pendingCameraSource) stop()                         {}
func (p *pendingCameraSource) startContext(ctx context.Context) (<-chan []byte, error) {
	close(p.requested)
	<-ctx.Done()
	close(p.cancelled)
	return nil, ctx.Err()
}

func TestCameraStopAndDisconnectCancelPermission(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		s := &pendingCameraSource{make(chan struct{}), make(chan struct{})}
		conn, reader := startCameraServe(t, s)
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		fmt.Fprintln(conn, `{"type":"start"}`)
		<-s.requested
		if disconnect {
			conn.Close()
		} else {
			fmt.Fprintln(conn, `{"type":"stop"}`)
			readCameraMessage(t, reader)
			if _, payload := readCameraMessage(t, reader); !bytes.Contains(payload, []byte("idle")) {
				t.Fatal("stop did not return to idle")
			}
		}
		select {
		case <-s.cancelled:
		case <-time.After(time.Second):
			t.Fatal("permission request was left open")
		}
	}
}

type reopeningCameraSource struct {
	requests                      atomic.Int32
	requested, cancelled, release chan struct{}
	frames                        chan []byte
}

func (s *reopeningCameraSource) start() (<-chan []byte, error) { panic("uncancellable start") }
func (s *reopeningCameraSource) stop()                         { close(s.frames) }
func (s *reopeningCameraSource) startContext(ctx context.Context) (<-chan []byte, error) {
	if s.requests.Add(1) == 1 {
		close(s.requested)
		<-ctx.Done()
		close(s.cancelled)
		<-s.release
		return nil, ctx.Err()
	}
	s.frames = make(chan []byte, 1)
	s.frames <- cameraBlackFrame()
	return s.frames, nil
}
func TestCameraReopensDuringPermissionCancellation(t *testing.T) {
	s := &reopeningCameraSource{requested: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{})}
	conn, reader := startCameraServe(t, s)
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprintln(conn, `{"type":"start"}`)
	<-s.requested
	fmt.Fprintln(conn, `{"type":"stop"}`)
	readCameraMessage(t, reader)
	readCameraMessage(t, reader)
	<-s.cancelled
	fmt.Fprintln(conn, `{"type":"start"}`)
	close(s.release)
	if _, payload := readCameraMessage(t, reader); !bytes.Contains(payload, []byte("streaming")) {
		t.Fatalf("reopen failed: %s", payload)
	}
	if kind, _ := readCameraMessage(t, reader); kind != cameraKindFrame {
		t.Fatal("reopen did not capture")
	}
}

func TestParsePipeWireCameraDevices(t *testing.T) {
	data := []byte(`[
        {"type":"PipeWire:Interface:Node","info":{"props":{"media.class":"Video/Source","node.name":"camera.2","node.description":"USB\nCamera","api.v4l2.path":"/dev/video2"}}},
        {"type":"PipeWire:Interface:Node","info":{"props":{"media.class":"Video/Source","node.name":"camera.0","node.description":"USB Camera","api.v4l2.path":"/dev/video0"}}},
        {"type":"PipeWire:Interface:Node","info":{"props":{"media.class":"Video/Source","node.name":"camera.0"}}},
        {"type":"PipeWire:Interface:Node","info":{"props":{"media.class":"Audio/Source","node.name":"microphone"}}}
    ]`)
	devices, err := parsePipeWireMediaDevices(data, "Video/Source")
	if err != nil {
		t.Fatal(err)
	}
	cameras := devices["Video/Source"]
	if len(cameras) != 2 || cameras[0].Name != "camera.0" || cameras[0].Label != "USB Camera (video0)" || cameras[1].Label != "USB Camera (video2)" {
		t.Fatalf("camera catalog: %+v", cameras)
	}
}

type testCameraPortal struct {
	conn   *dbus.Conn
	code   atomic.Uint32
	stall  atomic.Bool
	opened atomic.Int32
	closed chan struct{}
	file   *os.File
}

const testCameraRequest = dbus.ObjectPath("/org/freedesktop/portal/desktop/request/test/camera")

func (p *testCameraPortal) AccessCamera(options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	if _, ok := options["handle_token"].Value().(string); !ok {
		return "", dbus.MakeFailedError(os.ErrInvalid)
	}
	if !p.stall.Load() {
		go func() {
			time.Sleep(10 * time.Millisecond)
			p.conn.Emit(testCameraRequest, "org.freedesktop.portal.Request.Response", p.code.Load(), map[string]dbus.Variant{})
		}()
	}
	return testCameraRequest, nil
}
func (p *testCameraPortal) OpenPipeWireRemote(options map[string]dbus.Variant) (dbus.UnixFD, *dbus.Error) {
	p.opened.Add(1)
	return dbus.UnixFD(p.file.Fd()), nil
}
func (p *testCameraPortal) Close() *dbus.Error {
	select {
	case p.closed <- struct{}{}:
	default:
	}
	return nil
}

func TestLinuxCameraPortalPrivateBus(t *testing.T) {
	if os.Getenv("TRYOMARCHY_TEST_PRIVATE_BUS") != "1" {
		t.Skip("requires a test-owned session bus")
	}
	server, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if reply, err := server.RequestName(linuxPortalDesktop, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("private portal name: %v %v", reply, err)
	}
	file, err := os.CreateTemp(t.TempDir(), "remote")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	file.WriteString("restricted camera remote")
	file.Seek(0, io.SeekStart)
	p := &testCameraPortal{conn: server, file: file, closed: make(chan struct{}, 4)}
	server.Export(p, linuxPortalObject, "org.freedesktop.portal.Camera")
	server.Export(p, testCameraRequest, "org.freedesktop.portal.Request")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	fd, err := linuxCameraRemote(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(fd)
	fd.Close()
	if err != nil || string(content) != "restricted camera remote" || p.opened.Load() != 1 {
		t.Fatalf("remote fd: %q %v", content, err)
	}
	<-p.closed
	p.code.Store(1)
	if _, err := linuxCameraRemote(ctx); err == nil || p.opened.Load() != 1 {
		t.Fatalf("denied camera opened remote: %v", err)
	}
	<-p.closed
	p.stall.Store(true)
	short, shortCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer shortCancel()
	if _, err := linuxCameraRemote(short); err == nil {
		t.Fatal("permission timeout ignored")
	}
	select {
	case <-p.closed:
	case <-time.After(time.Second):
		t.Fatal("timed out portal request not closed")
	}
}
