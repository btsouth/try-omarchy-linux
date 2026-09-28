//go:build linux

package main

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
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
	if err := rejectQEMUImageLock(path, disk); err != nil {
		t.Fatalf("portal disk used byte locks the portal cannot report: %v", err)
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

func TestBackupRejectsQEMUHoldingImageLock(t *testing.T) {
	qemu, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		t.Skip("QEMU runtime unavailable")
	}
	path := filepath.Join(t.TempDir(), "disk.raw")
	if err := os.WriteFile(path, make([]byte, 16<<20), 0600); err != nil {
		t.Fatal(err)
	}
	startQEMU := func() *exec.Cmd {
		vm := exec.Command(qemu, "-machine", "none", "-nodefaults", "-S", "-display", "none", "-drive", "if=none,id=disk0,format=raw,file="+path)
		if err := vm.Start(); err != nil {
			t.Fatal(err)
		}
		return vm
	}
	vm := startQEMU()
	stop := func() { vm.Process.Kill(); vm.Wait() }
	defer stop()
	deadline := time.Now().Add(5 * time.Second)
	for {
		disk, err := openBackupDisk(path)
		if err != nil {
			if !strings.Contains(err.Error(), "still has the guest disk open") {
				t.Fatalf("unexpected rejection: %v", err)
			}
			break
		}
		disk.Close()
		if time.Now().After(deadline) {
			t.Fatal("backup opened a disk held by a running QEMU")
		}
		time.Sleep(50 * time.Millisecond)
	}
	stop()
	disk, err := openBackupDisk(path)
	if err != nil {
		t.Fatalf("stopped QEMU still blocks backup: %v", err)
	}
	defer disk.Close()
	// While the backup holds the disk, a new QEMU must not claim it.
	late := exec.Command(qemu, "-machine", "none", "-nodefaults", "-S", "-display", "none", "-drive", "if=none,id=disk0,format=raw,file="+path)
	done := make(chan error, 1)
	if err := late.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- late.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("QEMU exited cleanly instead of refusing the locked disk")
		}
	case <-time.After(5 * time.Second):
		late.Process.Kill()
		<-done
		t.Fatal("QEMU opened the disk while the backup held it")
	}
}
