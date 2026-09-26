//go:build linux

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func allocatedBlocks(t *testing.T, f *os.File) int64 {
	t.Helper()
	var st syscall.Stat_t
	if err := syscall.Fstat(int(f.Fd()), &st); err != nil {
		t.Fatal(err)
	}
	return st.Blocks * 512
}

func TestSparseCopyLeavesZeroBlocksUnallocated(t *testing.T) {
	dir := t.TempDir()
	src, err := os.Create(filepath.Join(dir, "rootfs.ext4"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	// 8 MiB image with data only in the first and last MiB.
	data := bytes.Repeat([]byte{0xa5}, 1<<20)
	if _, err := src.WriteAt(data, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := src.WriteAt(data, 7<<20); err != nil {
		t.Fatal(err)
	}
	dst, err := os.Create(filepath.Join(dir, "disk.raw"))
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if err := setSparse(dst); err != nil {
		t.Fatal(err)
	}
	if err := sparseCopy(dst, src, 8<<20, getUI()); err != nil {
		t.Fatal(err)
	}
	info, err := dst.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 8<<20 {
		t.Fatalf("copy size = %d, want %d", info.Size(), 8<<20)
	}
	if got := allocatedBlocks(t, dst); got > 3<<20 {
		t.Fatalf("copy allocates %d bytes; the six zero MiB should stay holes", got)
	}
	got := make([]byte, 1<<20)
	if _, err := dst.ReadAt(got, 7<<20); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("last MiB not copied: err=%v", err)
	}
}

func TestPunchHoleFreesSpaceAndReadsZeros(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "disk.raw"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(bytes.Repeat([]byte{0x5a}, 4<<20)); err != nil {
		t.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	before := allocatedBlocks(t, f)
	if err := punchHole(f, 1<<20, 2<<20); err != nil {
		if errors.Is(err, syscall.EOPNOTSUPP) {
			t.Skip("temporary directory's file system cannot punch holes")
		}
		t.Fatal(err)
	}
	if after := allocatedBlocks(t, f); after > before-(2<<20) {
		t.Fatalf("allocation %d -> %d, want at least 2 MiB freed", before, after)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 4<<20 {
		t.Fatalf("size changed to %d", info.Size())
	}
	got := make([]byte, 2<<20)
	if _, err := f.ReadAt(got, 1<<20); err != nil || !bytes.Equal(got, make([]byte, 2<<20)) {
		t.Fatalf("punched range does not read as zeros: err=%v", err)
	}
}
