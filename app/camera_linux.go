//go:build linux

package main

import (
	"fmt"
	"net"
)

// runCameraBridge answers QEMU's camera channel. The port has to be open
// before QEMU starts even without camera capture: QEMU 11.1 aborts at startup
// when a reconnecting client chardev finds nothing listening. Guest capture
// requests get the unavailable source's error.
func runCameraBridge(desktopPreferences) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cameraPort))
	if err != nil {
		fatal("Try Omarchy camera port %d is in use.", cameraPort)
	}
	logf("camera: bridge listening on %d (no capture on Linux yet)", cameraPort)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				logf("camera: accept: %v", err)
				return
			}
			go func() {
				defer conn.Close()
				if err := serveCamera(conn, newCameraFrameSource()); err != nil {
					logf("camera: %v", err)
				}
			}()
		}
	}()
}
