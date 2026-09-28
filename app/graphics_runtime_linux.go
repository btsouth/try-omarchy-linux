//go:build linux

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Only inspect a bounded tail. QEMU and Mesa may append warnings throughout
// a long session, but the virgl command/resource failure is actionable.
func linuxVirglDesktopError(vmDir string) bool {
	f, err := os.Open(filepath.Join(vmDir, "qemu-stderr.log"))
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false
	}
	const limit int64 = 128 << 10
	if info.Size() > limit {
		if _, err := f.Seek(info.Size()-limit, io.SeekStart); err != nil {
			return false
		}
	}
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return false
	}
	message := string(data)
	return strings.Contains(message, "virtio_gpu_virgl_process_cmd: ctrl 0x106, error 0x1200") ||
		(strings.Contains(message, "context error reported") &&
			(strings.Contains(message, "Illegal resource") || strings.Contains(message, "Illegal command buffer")))
}
