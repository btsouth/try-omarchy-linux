//go:build linux

package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var qemuPid atomic.Uint32

// Fixed loopback bridges accept only sockets owned by the current QEMU.
// Read that process's network namespace and descriptors, also inside Flatpak.
func connectionFromQEMU(conn net.Conn) bool {
	pid := qemuPid.Load()
	local, lok := conn.RemoteAddr().(*net.TCPAddr)
	remote, rok := conn.LocalAddr().(*net.TCPAddr)
	if pid == 0 || !lok || !rok || !local.IP.IsLoopback() || !remote.IP.IsLoopback() {
		return false
	}
	prefix := fmt.Sprintf("/proc/%d", pid)
	inodes := map[string]bool{}
	descriptors, err := os.ReadDir(prefix + "/fd")
	if err != nil {
		return false
	}
	for _, fd := range descriptors {
		target, err := os.Readlink(prefix + "/fd/" + fd.Name())
		if err == nil && strings.HasPrefix(target, "socket:[") {
			inodes[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = true
		}
	}
	for _, name := range []string{"tcp", "tcp6"} {
		data, err := os.ReadFile(prefix + "/net/" + name)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 10 || !inodes[fields[9]] {
				continue
			}
			if linuxSocketEndpointMatches(fields[1], local) && linuxSocketEndpointMatches(fields[2], remote) && qemuPid.Load() == pid {
				return true
			}
		}
	}
	return false
}

func linuxSocketEndpointMatches(encoded string, endpoint *net.TCPAddr) bool {
	parts := strings.Split(encoded, ":")
	if len(parts) != 2 {
		return false
	}
	port, err := strconv.ParseUint(parts[1], 16, 16)
	if err != nil || int(port) != endpoint.Port {
		return false
	}
	address := parts[0]
	if len(address) != 8 && len(address) != 32 {
		return false
	}
	ip := make(net.IP, len(address)/2)
	for i := 0; i < len(ip); i += 4 {
		word, err := strconv.ParseUint(address[i*2:i*2+8], 16, 32)
		if err != nil {
			return false
		}
		for j := 0; j < 4; j++ {
			ip[i+j] = byte(word >> (8 * j))
		}
	}
	return ip.Equal(endpoint.IP)
}

func lifecycleConnectionFromQEMU(conn net.Conn) bool {
	deadline := time.Now().Add(500 * time.Millisecond)
	for qemuPid.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	return connectionFromQEMU(conn)
}
