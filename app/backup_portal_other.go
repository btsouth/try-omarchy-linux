//go:build !windows && !linux

package main

import "os"

func portalBackupDiskLockFallback(_ string, _ *os.File, lockErr error) error { return lockErr }

func rejectQEMUImageLock(_ string, _ *os.File) error { return nil }
