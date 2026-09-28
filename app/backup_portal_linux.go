//go:build linux

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// The document portal does not implement flock. QEMU also runs without OFD
// locks on this mount. The launcher owns its single-instance lifecycle port
// during backup, and these checks reject an orphaned QEMU before reading.
func portalBackupDiskLockFallback(path string, disk *os.File, lockErr error) error {
	if !linuxDocumentPortalDisk(path, os.Getuid()) ||
		(!errors.Is(lockErr, syscall.ENOSYS) && !errors.Is(lockErr, syscall.EOPNOTSUPP)) {
		return lockErr
	}
	control, err := qmpControlPath(qmpSupPort)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", control, 300*time.Millisecond)
	if err == nil {
		conn.Close()
		return fmt.Errorf("a virtual machine is still using its private control socket")
	}
	if !errors.Is(err, os.ErrNotExist) && !qmpConnectionRefused(err) {
		return fmt.Errorf("cannot verify that the virtual machine stopped: %w", err)
	}
	return rejectQEMUWithOpenDisk(disk)
}

func rejectQEMUWithOpenDisk(disk *os.File) error {
	want, err := disk.Stat()
	if err != nil {
		return err
	}
	processes, err := os.ReadDir("/proc")
	if err != nil {
		return fmt.Errorf("cannot inspect running virtual machines: %w", err)
	}
	for _, process := range processes {
		if _, err := strconv.Atoi(process.Name()); err != nil {
			continue
		}
		root := filepath.Join("/proc", process.Name())
		comm, err := os.ReadFile(filepath.Join(root, "comm"))
		if err != nil || !strings.HasPrefix(strings.TrimSpace(string(comm)), "qemu-system") {
			continue
		}
		fds, err := os.ReadDir(filepath.Join(root, "fd"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("cannot inspect running virtual machine %s: %w", process.Name(), err)
		}
		for _, fd := range fds {
			info, err := os.Stat(filepath.Join(root, "fd", fd.Name()))
			if err == nil && os.SameFile(want, info) {
				return fmt.Errorf("a virtual machine still has the guest disk open")
			}
		}
	}
	return nil
}

// flock never conflicts with the OFD byte locks QEMU uses to mark an image in
// use. Hold a shared lock on the whole disk so a QEMU starting during the
// backup cannot claim it, then test for a lock from any other open file.
// The document portal keeps byte locks local and fails the test with EIO, so
// its disks rely on the fallback checks and the unchanged-disk check instead.
func rejectQEMUImageLock(path string, disk *os.File) error {
	if linuxDocumentPortalDisk(path, os.Getuid()) {
		return nil
	}
	lock := syscall.Flock_t{Type: syscall.F_RDLCK, Whence: 0}
	if err := ofdLock(disk, linuxFOFDSetLock, &lock); err != nil {
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EACCES) {
			return fmt.Errorf("a virtual machine still has the guest disk open")
		}
		return fmt.Errorf("cannot lock the guest disk: %w", err)
	}
	lock = syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0}
	if err := ofdLock(disk, linuxFOFDGetLock, &lock); err != nil {
		return fmt.Errorf("cannot check whether the guest disk is in use: %w", err)
	}
	if lock.Type != syscall.F_UNLCK {
		return fmt.Errorf("a virtual machine still has the guest disk open")
	}
	return nil
}

const (
	linuxFOFDGetLock = 36
	linuxFOFDSetLock = 37
)

func ofdLock(f *os.File, command int, lock *syscall.Flock_t) error {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), uintptr(command), uintptr(unsafe.Pointer(lock)))
	if errno != 0 {
		return errno
	}
	return nil
}
