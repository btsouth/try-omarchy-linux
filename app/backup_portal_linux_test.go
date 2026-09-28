//go:build linux

package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestPortalBackupDiskLockFallback(t *testing.T) {
	previous := qmpControlDirectory
	controlDir := t.TempDir()
	qmpControlDirectory = func() (string, error) { return controlDir, nil }
	t.Cleanup(func() { qmpControlDirectory = previous })
	disk, err := os.CreateTemp(t.TempDir(), "disk-*")
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	path := filepath.Join("/run/user", strconv.Itoa(os.Getuid()), "doc", "grant", "vm", "disk.raw")
	if err := portalBackupDiskLockFallback(path, disk, syscall.ENOSYS); err != nil {
		t.Fatalf("portal fallback: %v", err)
	}
	if err := portalBackupDiskLockFallback(path, disk, syscall.EWOULDBLOCK); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("busy lock was ignored: %v", err)
	}
	if err := portalBackupDiskLockFallback(disk.Name(), disk, syscall.ENOSYS); !errors.Is(err, syscall.ENOSYS) {
		t.Fatalf("nonportal disk bypassed locking: %v", err)
	}
	control, err := qmpControlPath(qmpSupPort)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", control)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := portalBackupDiskLockFallback(path, disk, syscall.ENOSYS); err == nil || !strings.Contains(err.Error(), "still using") {
		t.Fatalf("active QMP socket was ignored: %v", err)
	}
}
