//go:build linux

package main

import (
	"context"
	"errors"
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

// openLinuxFolder uses the OpenURI portal inside Flatpak and xdg-open
// outside it.
func openLinuxFolder(path string) error {
	if inFlatpak() {
		return openLinuxFolderThroughPortal(path)
	}
	cmd := exec.Command("xdg-open", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// openLinuxFolderThroughPortal passes a descriptor, which works for any folder
// the sandbox can read, including one granted through the document portal,
// without exposing other host paths. It waits for the portal's answer, since
// the call itself only starts the request.
func openLinuxFolderThroughPortal(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if info, err := dir.Stat(); err != nil || !info.IsDir() {
		return uiError(uiTextWith("tray.linux.is_not_a_folder", map[string]string{"path": path}), nil)
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	if err := conn.AddMatchSignal(dbus.WithMatchSender(linuxPortalDesktop), dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")); err != nil {
		return err
	}
	// The desktop may ask which app to use.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	token := fmt.Sprintf("tryomarchy_%d", time.Now().UnixNano())
	_, err = linuxPortalRequest(ctx, conn, signals, "org.freedesktop.portal.OpenURI.OpenFile",
		"", dbus.UnixFD(dir.Fd()), map[string]dbus.Variant{"handle_token": dbus.MakeVariant(token)})
	if errors.Is(err, errLinuxClipboardDenied) {
		return errors.New(uiText("tray.linux.the_desktop_did_not_open_it"))
	}
	return err
}

// openLinuxSharedFolder opens the folder shared with the running VM. A
// folder chosen in Settings since launch is shared only from the next launch.
func openLinuxSharedFolder(share string) {
	if share == "" {
		tellLinuxUser("share", uiText("tray.linux.no_shared_folder"), uiText("tray.linux.omarchy_was_started_without_a_shared_folder_choose"))
		return
	}
	if err := linuxOpenFolder(share); err != nil {
		logf("tray: open shared folder: %v", err)
		tellLinuxUser("share", uiText("tray.linux.could_not_open_the_shared_folder"), uiTextWith("tray.linux.try_omarchy_could_not_open", map[string]string{"path": linuxSharedFolderDisplayPath(share), "error": err.Error()}))
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
		tellLinuxUser("diagnostics", uiText("tray.linux.diagnostics_failed"), uiTextWith("tray.linux.could_not_create_diagnostics", map[string]string{"error": err.Error()}))
		return
	}
	tellLinuxUser("diagnostics", uiText("tray.linux.diagnostics_saved"), uiTextWith("tray.linux.saved_to_review_the_bundle_before_sharing_it", map[string]string{"path": path}))
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
	state.Primary = uiText("launcher.close")
	w.ask(ctx, state)
}
