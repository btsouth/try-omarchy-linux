//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

func newCameraFrameSource() cameraFrameSource { return &linuxCameraSource{} }

func configuredCameraSource(p desktopPreferences) cameraFrameSource {
	if p.CameraDisabled {
		return disabledCameraSource{}
	}
	return &linuxCameraSource{deviceID: p.CameraID}
}

// The portal owns permission. Its restricted PipeWire socket is passed only
// to the capture helper, never substituted with the host's unrestricted one.
func linuxCameraRemote(ctx context.Context) (*os.File, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, uiError(uiTextWith("camera.linux.camera_permission_service_is_unavailable", map[string]string{"error": err.Error()}), err)
	}
	defer conn.Close()
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	if err = conn.AddMatchSignal(dbus.WithMatchSender(linuxPortalDesktop), dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")); err != nil {
		return nil, err
	}
	var request dbus.ObjectPath
	options := map[string]dbus.Variant{"handle_token": dbus.MakeVariant(fmt.Sprintf("trycamera_%d", time.Now().UnixNano()))}
	if err = conn.Object(linuxPortalDesktop, linuxPortalObject).CallWithContext(ctx, "org.freedesktop.portal.Camera.AccessCamera", 0, options).Store(&request); err != nil {
		return nil, uiError(uiTextWith("camera.linux.could_not_request_camera_permission", map[string]string{"error": err.Error()}), err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = conn.Object(linuxPortalDesktop, request).CallWithContext(closeCtx, "org.freedesktop.portal.Request.Close", 0).Err
	}()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case signal, ok := <-signals:
			if !ok {
				return nil, errors.New(uiText("camera.linux.camera_permission_service_closed"))
			}
			if signal == nil || signal.Path != request || len(signal.Body) != 2 {
				continue
			}
			code, valid := signal.Body[0].(uint32)
			if !valid {
				return nil, errors.New(uiText("camera.linux.invalid_camera_permission_response"))
			}
			if code != 0 {
				return nil, errors.New(uiText("camera.linux.camera_access_was_not_granted_by_your_desktop"))
			}
			var fd dbus.UnixFD
			if err = conn.Object(linuxPortalDesktop, linuxPortalObject).CallWithContext(ctx, "org.freedesktop.portal.Camera.OpenPipeWireRemote", 0, map[string]dbus.Variant{}).Store(&fd); err != nil {
				return nil, uiError(uiTextWith("camera.linux.could_not_open_the_camera_connection", map[string]string{"error": err.Error()}), err)
			}
			if fd < 0 {
				return nil, errors.New(uiText("camera.linux.invalid_camera_connection"))
			}
			return os.NewFile(uintptr(fd), "camera-portal"), nil
		}
	}
}

// At most a short diagnostic is retained. Camera frames are never logged.
type cameraDiagnostic struct {
	mu   sync.Mutex
	text string
}

func (d *cameraDiagnostic) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if left := 2048 - len(d.text); left > 0 {
		d.text += string(p[:min(left, len(p))])
	}
	return len(p), nil
}
func (d *cameraDiagnostic) error() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if text := strings.TrimSpace(d.text); text != "" {
		return uiError(uiTextWith("camera.linux.capture_failed", map[string]string{"error": text}), nil)
	}
	return errors.New(uiText("camera.linux.camera_capture_ended_before_a_frame_arrived"))
}

type linuxCameraSource struct {
	deviceID string
	remote   func(context.Context) (*os.File, error)
	helper   string
	cancel   context.CancelFunc
	finished chan struct{}
}

func (s *linuxCameraSource) start() (<-chan []byte, error) {
	return s.startContext(context.Background())
}

func (s *linuxCameraSource) startContext(parent context.Context) (<-chan []byte, error) {
	ctx, cancel := context.WithCancel(parent)
	permissionCtx, permissionCancel := context.WithTimeout(ctx, 90*time.Second)
	remote := s.remote
	if remote == nil {
		remote = linuxCameraRemote
	}
	fd, err := remote(permissionCtx)
	permissionCancel()
	if err != nil {
		cancel()
		return nil, err
	}
	defer fd.Close()
	helper := s.helper
	if helper == "" {
		helper = "try-omarchy-camera"
	}
	cmd := exec.CommandContext(ctx, helper, s.deviceID)
	// A crash or forced launcher exit must not leave a camera process running.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	cmd.WaitDelay = time.Second
	cmd.ExtraFiles = []*os.File{fd}
	diagnostic := &cameraDiagnostic{}
	cmd.Stderr = diagnostic
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		cancel()
		stdout.Close()
		return nil, uiError(uiTextWith("camera.linux.could_not_start_camera_capture", map[string]string{"error": err.Error()}), err)
	}
	s.cancel, s.finished = cancel, make(chan struct{})
	finished := s.finished
	frames := make(chan []byte, 2)
	ready := make(chan struct{})
	go func() {
		defer close(finished)
		defer close(frames)
		defer cancel()
		first := true
		for {
			frame := make([]byte, cameraFrameBytes)
			if _, err := io.ReadFull(stdout, frame); err != nil {
				break
			}
			select {
			case frames <- frame:
				if first {
					close(ready)
					first = false
				}
			case <-ctx.Done():
				cancel()
				_ = stdout.Close()
				_ = cmd.Wait()
				return
			default: // drop when the guest falls behind
			}
		}
		// A helper that closes stdout but stays alive must not hold capture.
		cancel()
		_ = cmd.Wait()
	}()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case <-ready:
		return frames, nil
	case <-s.finished:
		s.stop()
		return nil, diagnostic.error()
	case <-parent.Done():
		s.stop()
		return nil, parent.Err()
	case <-timer.C:
		s.stop()
		return nil, errors.New(uiText("camera.linux.the_camera_did_not_provide_a_frame_close_other_camera_apps_and_try_again"))
	}
}

func (s *linuxCameraSource) stop() {
	if s.cancel != nil {
		s.cancel()
		<-s.finished
		s.cancel, s.finished = nil, nil
	}
}
