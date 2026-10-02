//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

func rejectMoveLink(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("linked paths cannot be moved: %s", path)
	}
	return nil
}
func rejectAncestorLink(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 && runtime.GOOS == "linux" {
		target, err := os.Readlink(path)
		if err == nil && linuxAtomicHomeAncestor(path, target) {
			// Atomic desktops and Flatpak use this system alias. Validate its
			// destination too; links within the user's data remain forbidden.
			if err := validateMovePath("/var/home"); err != nil {
				return err
			}
			if home, err := os.Stat("/var/home"); err == nil && home.IsDir() {
				return nil
			}
		}
		if err == nil && linuxPortalMountAncestor(path, target, os.Getuid()) {
			if mount, err := os.Stat("/run/flatpak/doc"); err == nil && mount.IsDir() {
				return nil
			}
		}
	}
	return rejectMoveLink(path, info)
}

// Fedora Atomic desktops (including Bazzite) put home directories in /var/home.
// Flatpak preserves /home -> var/home inside its sandbox. Only this exact system
// ancestor is allowed, never a selected move target or a link within user data.
func linuxAtomicHomeAncestor(path, target string) bool {
	return path == "/home" && filepath.Clean(filepath.Join("/", target)) == "/var/home"
}

// Flatpak's document portal places this one link in the private runtime tree.
// The grant, selected folder, VM directory and disk still pass the usual link
// checks; arbitrary links within the installation remain forbidden.
func linuxPortalMountAncestor(path, target string, uid int) bool {
	root := fmt.Sprintf("/run/user/%d", uid)
	rel, err := filepath.Rel(root, filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) &&
		filepath.Base(path) == "doc" &&
		filepath.Clean(filepath.Join(filepath.Dir(path), target)) == "/run/flatpak/doc"
}
func publishMoveFile(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	f, err := os.Open(filepath.Dir(to))
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func publishMoveDirectory(from, to string) error { return publishMoveFile(from, to) }
func lockMoveStore(s moveStore) (*os.File, error) {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(s.dir, "mutation.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if portalStoreLockFallback(s.dir, err) {
			return f, nil
		}
		f.Close()
		return nil, err
	}
	return f, nil
}

func openMoveCleanupDisk(path string) (*os.File, error) { return openBackupDisk(path) }
