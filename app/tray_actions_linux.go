//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// Tray actions handled by the running session's supervisor, which knows the
// VM folder and the folder shared with this launch.
var (
	linuxShareRequests       = make(chan struct{}, 1)
	linuxDiagnosticsRequests = make(chan struct{}, 1)
	linuxHelpRequests        = make(chan struct{}, 1)
	linuxHelpOpen            atomic.Bool
	linuxDiagnosticsRunning  atomic.Bool
)

func requestLinuxTrayAction(requests chan struct{}) {
	select {
	case requests <- struct{}{}:
	default:
	}
}

// linuxOpenFolder opens a folder in the desktop's file manager. It is a
// variable so tests can watch it.
var linuxOpenFolder = openLinuxFolder

// openLinuxFolder uses the OpenURI portal inside Flatpak. Passing a descriptor
// works for any folder the sandbox can read, including one granted through
// the document portal, without exposing other host paths.
func openLinuxFolder(path string) error {
	if !inFlatpak() {
		cmd := exec.Command("xdg-open", path)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return err
		}
		go cmd.Wait()
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if info, err := dir.Stat(); err != nil || !info.IsDir() {
		return fmt.Errorf("%s is not a folder", path)
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var handle dbus.ObjectPath
	return conn.Object(linuxPortalDesktop, linuxPortalObject).CallWithContext(ctx, "org.freedesktop.portal.OpenURI.OpenFile", 0,
		"", dbus.UnixFD(dir.Fd()), map[string]dbus.Variant{}).Store(&handle)
}

// openLinuxSharedFolder opens the folder shared with the running VM. A
// folder chosen in Settings since launch is shared only from the next launch.
func openLinuxSharedFolder(share string) {
	if share == "" {
		tellLinuxUser("share", "No shared folder", "Omarchy was started without a shared folder. Choose one in Settings; it is shared the next time Omarchy starts.")
		return
	}
	if err := linuxOpenFolder(share); err != nil {
		logf("tray: open shared folder: %v", err)
		tellLinuxUser("share", "Could not open the shared folder", "Try Omarchy could not open "+linuxSharedFolderDisplayPath(share)+": "+err.Error())
	}
}

// createLinuxDiagnostics writes a diagnostics bundle into the VM folder and
// says where it is.
func createLinuxDiagnostics(dir string) {
	if !linuxDiagnosticsRunning.CompareAndSwap(false, true) {
		return
	}
	defer linuxDiagnosticsRunning.Store(false)
	facts := hostFacts()
	facts["launcher.version"] = linuxAppVersion
	facts["time"] = time.Now().Format(time.RFC3339)
	path, err := writeDiagnostics(dir, facts)
	if err != nil {
		tellLinuxUser("diagnostics", "Diagnostics failed", "Could not create diagnostics: "+err.Error())
		return
	}
	tellLinuxUser("diagnostics", "Diagnostics saved", "Saved to "+path+". Review the bundle before sharing it; logs can still contain local details.")
}

// showLinuxHelp opens the help and shortcuts page in its own window.
func showLinuxHelp(parent context.Context) {
	if !linuxHelpOpen.CompareAndSwap(false, true) {
		return
	}
	defer linuxHelpOpen.Store(false)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	w := startLinuxWindow(cancel)
	if w == nil {
		return
	}
	defer w.stop()
	state := linuxAboutState()
	state.Primary = "Close"
	w.ask(ctx, state)
}
