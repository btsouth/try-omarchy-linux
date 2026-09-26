//go:build linux

package main

import (
	"os"
	"syscall"
)

// Linux file systems leave a hole wherever a writer seeks past unwritten
// blocks, so there is nothing to switch on before a sparse copy.
func setSparse(*os.File) error { return nil }

const (
	fallocKeepSize  = 0x01 // FALLOC_FL_KEEP_SIZE
	fallocPunchHole = 0x02 // FALLOC_FL_PUNCH_HOLE
)

// punchHole deallocates a zero range of a disk image so it stops costing
// space. Reads of the range still return zeros. File systems without hole
// punching report EOPNOTSUPP, which the caller surfaces.
func punchHole(f *os.File, offset, length int64) error {
	return syscall.Fallocate(int(f.Fd()), fallocPunchHole|fallocKeepSize, offset, length)
}
