package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
)

// captureNative asks the isolated test desktop for a screenshot of the window
// as the compositor shows it, when TRYOMARCHY_UI_CAPTURE names a folder. It
// writes name.request there and waits briefly for name.png from a watcher
// outside the sandbox. Captures are evidence, not checks.
func captureNative(_ *adw.ApplicationWindow, name string) error {
	dir := os.Getenv("TRYOMARCHY_UI_CAPTURE")
	if dir == "" {
		return nil
	}
	if err := os.WriteFile(filepath.Join(dir, name+".request"), nil, 0o666); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, name+".png")); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("no screenshot arrived")
}
