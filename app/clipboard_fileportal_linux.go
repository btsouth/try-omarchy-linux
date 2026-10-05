//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const linuxDocumentsPortal = "org.freedesktop.portal.Documents"
const linuxDocumentsObject = dbus.ObjectPath("/org/freedesktop/portal/documents")

func (p *linuxPortalClipboard) stopFileTransfer(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = p.conn.Object(linuxDocumentsPortal, linuxDocumentsObject).CallWithContext(ctx, "org.freedesktop.portal.FileTransfer.StopTransfer", 0, key).Err
}

func (p *linuxPortalClipboard) setPaths(paths []string) bool {
	if err := p.publishFiles(paths); err != nil {
		logf("portal clipboard files: %v", err)
		setLinuxClipboardStatus(uiTextWith("settings.clipboard.linux.share_files_error", map[string]string{"error": err.Error()}), true)
		return false
	}
	return true
}

// The document portal exports the app's private received files to the next
// clipboard consumer. A URI list would expose paths that other sandboxes
// cannot open. Keep the transfer alive until the selection changes or closes.
func (p *linuxPortalClipboard) publishFiles(paths []string) error {
	if len(paths) == 0 || len(paths) > 1000 {
		return errors.New("invalid file selection")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	documents := p.conn.Object(linuxDocumentsPortal, linuxDocumentsObject)
	var key string
	if err := documents.CallWithContext(ctx, "org.freedesktop.portal.FileTransfer.StartTransfer", 0, map[string]dbus.Variant{}).Store(&key); err != nil {
		return fmt.Errorf("file transfer portal unavailable: %w", err)
	}
	if key == "" || len(key) > 4096 || strings.ContainsRune(key, 0) {
		if key != "" {
			p.stopFileTransfer(key)
		}
		return errors.New("file transfer portal returned an invalid key")
	}
	defer func() {
		if key != "" {
			p.stopFileTransfer(key)
		}
	}()
	// Session buses commonly limit the number of FDs in one message.
	for start := 0; start < len(paths); start += 8 {
		end := min(start+8, len(paths))
		files := make([]*os.File, 0, end-start)
		fds := make([]dbus.UnixFD, 0, end-start)
		for _, path := range paths[start:end] {
			if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
				closeLinuxPortalFiles(files)
				return errors.New("invalid file path")
			}
			file, err := os.Open(path)
			if err != nil {
				closeLinuxPortalFiles(files)
				return fmt.Errorf("cannot open %q: %w", filepath.Base(path), err)
			}
			files = append(files, file)
			fds = append(fds, dbus.UnixFD(file.Fd()))
		}
		var err error
		if p.addFiles != nil {
			err = p.addFiles(ctx, key, fds)
		} else {
			err = documents.CallWithContext(ctx, "org.freedesktop.portal.FileTransfer.AddFiles", 0, key, fds, map[string]dbus.Variant{}).Err
		}
		closeLinuxPortalFiles(files)
		if err != nil {
			return fmt.Errorf("cannot grant copied files: %w", err)
		}
	}
	if err := p.setSelection("application/vnd.portal.filetransfer", []byte(key), key); err != nil {
		return err
	}
	key = ""
	return nil
}

func closeLinuxPortalFiles(files []*os.File) {
	for _, file := range files {
		_ = file.Close()
	}
}
