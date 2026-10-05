//go:build linux

package main

import (
	"net"
	"os"
	"testing"
)

func TestLinuxBridgeChecksSocketOwnerAndRuntime(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "[::1]:0"} {
		t.Run(address, func(t *testing.T) {
			listener, err := net.Listen("tcp", address)
			if err != nil {
				t.Skip(err)
			}
			defer listener.Close()
			client, err := net.Dial("tcp", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			accepted, err := listener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer accepted.Close()
			before := qemuPid.Load()
			defer qemuPid.Store(before)
			qemuPid.Store(uint32(os.Getpid()))
			if !connectionFromQEMU(accepted) {
				t.Fatal("current runtime socket refused")
			}
			qemuPid.Store(0)
			if connectionFromQEMU(accepted) {
				t.Fatal("accepted with no runtime")
			}
			qemuPid.Store(uint32(os.Getppid()))
			if connectionFromQEMU(accepted) {
				t.Fatal("foreign process accepted")
			}
		})
	}
}
