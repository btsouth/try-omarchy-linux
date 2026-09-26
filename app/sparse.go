package main

import (
	"bytes"
	"io"
	"os"
)

// sparseCopy skips all-zero 1 MiB blocks, so seeking past them leaves holes
// (setSparse prepares the file on Windows; Linux file systems do it on their
// own) and the factory image lands at its real data size instead of a full
// 6 GiB.
func sparseCopy(dst *os.File, src *os.File, total int64, ui *progressUI) error {
	buf := make([]byte, 1<<20)
	zero := make([]byte, 1<<20)
	var off int64
	for {
		if err := checkSetupCancelled(); err != nil {
			return err
		}
		n, err := io.ReadFull(src, buf)
		if n > 0 {
			if bytes.Equal(buf[:n], zero[:n]) {
				off += int64(n)
			} else {
				if _, werr := dst.WriteAt(buf[:n], off); werr != nil {
					return werr
				}
				off += int64(n)
			}
			if ui != nil {
				ui.setProgress(off, total)
			}
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return dst.Truncate(off)
		}
		if err != nil {
			return err
		}
	}
}
