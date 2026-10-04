//go:build linux

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

func linuxCameraSocketPath() (string, error) {
	return linuxPrivateSocketPath("camera.sock")
}

// linuxPrivateSocketPath names a socket in the private control directory.
func linuxPrivateSocketPath(name string) (string, error) {
	dir, err := qmpControlDirectory()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if !filepath.IsAbs(path) || len([]byte(path)) > 103 {
		return "", fmt.Errorf("private connection path %s is too long or is not absolute", name)
	}
	return path, nil
}

func listenLinuxCamera() (net.Listener, error) {
	return listenLinuxPrivateSocket("camera.sock")
}

// listenLinuxPrivateSocket listens on a socket only this user can reach, in
// a private directory, replacing a stale socket but not a live one.
func listenLinuxPrivateSocket(name string) (net.Listener, error) {
	path, err := linuxPrivateSocketPath(name)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err = validateMovePath(dir); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("private connection path %s contains another file", name)
		}
		conn, dialErr := net.DialTimeout("unix", path, 300*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			return nil, fmt.Errorf("another Omarchy runtime owns the private connection %s", name)
		}
		if !qmpConnectionRefused(dialErr) && !os.IsNotExist(dialErr) {
			return nil, dialErr
		}
		if err = os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(statErr) {
		return nil, statErr
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

// The private filesystem socket cannot be reached through another VM's
// loopback network, or by other users on this host.
func cameraChardev(dir string) string {
	return "socket,id=cam0,path=" + qemuOptionValue(filepath.Join(dir, "camera.sock")) + ",reconnect-ms=1000"
}

// runCameraBridge answers QEMU's camera channel. The port has to be open
// before QEMU starts even without camera capture: QEMU 11.1 aborts at startup
// when a reconnecting client chardev finds nothing listening. Guest capture
// requests start capture through the desktop camera portal.
func runCameraBridge(preferences desktopPreferences) {
	if preferences.CameraDisabled {
		cameraState.Store("Camera access is off. Enable it in Settings and launch Omarchy again.")
	}
	listener, err := listenLinuxCamera()
	if err != nil {
		fatal("Could not prepare the private camera connection: %v", err)
	}
	logf("camera: bridge listening on its private connection")
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				logf("camera: accept: %v", err)
				return
			}
			go func() {
				defer conn.Close()
				if err := serveCamera(conn, configuredCameraSource(preferences)); err != nil {
					logf("camera: %v", err)
				}
			}()
		}
	}()
}
