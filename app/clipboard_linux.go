//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

// The native utilities own selections; no shell interprets clipboard contents.
// Probe data-control first so wl-clipboard never uses its focus-stealing
// fallback on a compositor that does not expose background clipboard access.
var linuxClipboardStatus atomic.Value

func setLinuxClipboardStatus(message string, visible bool) {
	linuxClipboardStatus.Store(message)
	if visible {
		showLinuxRuntimeError(uiText("settings.clipboard.linux.clipboard_access"), message)
	}
}

type linuxClipboard struct {
	mu     sync.Mutex
	item   clipItem
	paths  []string
	key    [32]byte
	serial uint32
	portal *linuxPortalClipboard
	owner  linuxSelectionOwner
}

func clipboardCommand(input []byte, limit int64, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(out, limit+1))
	if readErr != nil || int64(len(data)) > limit {
		cmd.Process.Kill()
	}
	err = cmd.Wait()
	if readErr != nil {
		return nil, readErr
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("clipboard exceeds size limit")
	}
	return data, err
}

func linuxClipboardMIME(types string) string {
	has := func(mime string) bool {
		for _, line := range strings.Split(types, "\n") {
			if strings.TrimSpace(line) == mime {
				return true
			}
		}
		return false
	}
	if has("x-kde-passwordManagerHint") {
		return ""
	}
	for _, mime := range []string{"application/vnd.portal.filetransfer", "text/uri-list", "image/png", "text/plain;charset=utf-8", "text/plain", "UTF8_STRING"} {
		if has(mime) {
			return mime
		}
	}
	return ""
}

func linuxClipboardPaths(data []byte) ([]string, bool) {
	if len(data) > 131072 {
		return nil, false
	}
	var paths []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u, err := url.Parse(line)
		if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !filepath.IsAbs(u.Path) || strings.ContainsRune(u.Path, 0) {
			return nil, false
		}
		paths = append(paths, u.Path)
		if len(paths) > 1000 {
			return nil, false
		}
	}
	return paths, len(paths) > 0
}

func linuxClipboardRead(mime string) ([]byte, error) {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		if mime == "" {
			mime = "TARGETS"
		}
		return clipboardCommand(nil, maxClipboardImageBytes, "xclip", "-selection", "clipboard", "-out", "-target", mime)
	}
	if mime == "" {
		return clipboardCommand(nil, 65536, "wl-paste", "--list-types")
	}
	return clipboardCommand(nil, maxClipboardImageBytes, "wl-paste", "--no-newline", "--type", mime)
}

func (c *linuxClipboard) sequence() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.portal != nil {
		return c.serial
	}
	types, err := linuxClipboardRead("")
	mime := linuxClipboardMIME(string(types))
	var data []byte
	if err == nil && mime != "" {
		data, err = linuxClipboardRead(mime)
	}
	if err != nil {
		// Never replay cached content after the selection becomes unavailable.
		if c.item.Kind != "" || len(c.paths) != 0 {
			c.item, c.paths, c.key = clipItem{}, nil, [32]byte{}
			c.serial++
		}
		return c.serial
	}
	key := sha256.Sum256(append([]byte(mime+"\x00"), data...))
	if key == c.key {
		return c.serial
	}
	c.key, c.item, c.paths = key, clipItem{}, nil
	c.serial++
	switch mime {
	case "application/vnd.portal.filetransfer":
		var grantErr error
		c.paths, grantErr = linuxPortalClipboardFiles(string(data))
		if grantErr != nil {
			setLinuxClipboardStatus(uiTextWith("settings.clipboard.linux.access_files_error", map[string]string{"error": grantErr.Error()}), true)
		}
	case "text/uri-list":
		c.paths, _ = linuxClipboardPaths(data)
	case "image/png":
		c.item = pngItem(data)
	case "text/plain;charset=utf-8", "text/plain", "UTF8_STRING":
		c.item = textItem(string(data))
	}
	return c.serial
}

func linuxPortalClipboardFiles(key string) ([]string, error) {
	key = strings.TrimRight(key, "\x00\r\n")
	if key == "" || len(key) > 4096 || strings.ContainsRune(key, 0) {
		return nil, fmt.Errorf("invalid file transfer key")
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("file transfer portal unavailable: %w", err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var paths []string
	err = conn.Object("org.freedesktop.portal.Documents", "/org/freedesktop/portal/documents").CallWithContext(ctx, "org.freedesktop.portal.FileTransfer.RetrieveFiles", 0, key, map[string]dbus.Variant{}).Store(&paths)
	if err != nil {
		logf("clipboard file portal: %v", err)
		return nil, fmt.Errorf("file transfer grant failed: %w", err)
	}
	if len(paths) == 0 || len(paths) > 1000 {
		return nil, fmt.Errorf("file transfer portal returned no usable files")
	}
	for i, path := range paths {
		canonical, err := linuxTransferSource(path)
		if err != nil {
			return nil, fmt.Errorf("cannot open copied file: %w", err)
		}
		paths[i] = canonical
	}
	return paths, nil
}

func (c *linuxClipboard) get() (clipItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.item, c.item.allowed()
}
func (c *linuxClipboard) getPaths() ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.paths...), len(c.paths) > 0
}
func (c *linuxClipboard) set(item clipItem) bool {
	if !item.allowed() {
		return false
	}
	mime := "text/plain;charset=utf-8"
	if item.Kind == clipPNG {
		mime = "image/png"
	} else if item.Kind != clipText {
		return false
	}
	if c.portal != nil {
		return c.portal.set(mime, item.Data)
	}
	return c.owner.copy(mime, item.Data)
}
func (c *linuxClipboard) setPaths(paths []string) bool {
	var text strings.Builder
	for _, path := range paths {
		if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
			return false
		}
		u := url.URL{Scheme: "file", Path: path}
		text.WriteString(u.String() + "\r\n")
	}
	if len(paths) == 0 {
		return false
	}
	if c.portal != nil {
		return c.portal.setPaths(paths)
	}
	return c.owner.copy("text/uri-list", []byte(text.String()))
}

func runLinuxClipboardBridge() func() {
	supported := true
	c := &linuxClipboard{}
	names := []string{"wl-copy", "wl-paste"}
	if linuxClipboardSharingOff() {
		setLinuxClipboardStatus(linuxClipboardOffMessage, false)
		logf("clipboard: sharing is off; file drops remain available")
		supported = false
		names = nil
	} else if os.Getenv("WAYLAND_DISPLAY") == "" {
		if os.Getenv("DISPLAY") == "" {
			logf("clipboard: no graphical session available")
			return func() {}
		}
		names = []string{"xclip"}
	} else if linuxGNOMEWayland() {
		var ask func(linuxSetupState) (string, error)
		if getUI().window != nil {
			ask = func(state linuxSetupState) (string, error) { return getUI().window.ask(setupContext(), state) }
		}
		if !linuxClipboardConsent(ask) {
			setLinuxClipboardStatus(linuxClipboardOffMessage, false)
			logf("clipboard: sharing is off; file drops remain available")
			supported = false
		} else if portal, err := linuxPortalClipboardSession(c); err != nil {
			rememberLinuxClipboardDenial(err)
			message := uiTextWith("settings.clipboard.linux.denied", map[string]string{"help": linuxClipboardHowToTurnOn})
			if !errors.Is(err, errLinuxClipboardDenied) {
				message = uiText("settings.clipboard.linux.timed_out")
			}
			// Declining an optional permission is a choice, not an error.
			// The explanation already says how to enable it later.
			setLinuxClipboardStatus(message, !errors.Is(err, errLinuxClipboardDenied))
			logf("clipboard: GNOME portal: %v", err)
			supported = false
		} else {
			c.portal = portal
			setLinuxClipboardStatus(uiText("settings.clipboard.linux.gnome_granted"), false)
		}
		names = nil
	} else if _, err := clipboardCommand(nil, 128, "try-omarchy-clipboard-capabilities"); err != nil {
		message := uiText("settings.clipboard.linux.unavailable")
		// Missing optional compositor support is a capability notice, not a
		// runtime error. A separate window disrupts tiling desktops at startup.
		setLinuxClipboardStatus(message, false)
		logf("clipboard: %s", message)
		supported = false
	}
	if setupCancelled() {
		return func() {}
	}
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			logf("clipboard: %s unavailable", name)
			supported = false
		}
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		logf("clipboard: %v", err)
		return func() {}
	}
	cache = filepath.Join(cache, "try-omarchy", "transfers", "streaming")
	if err := validateMovePath(cache); err != nil {
		logf("clipboard: %v", err)
		return func() {}
	}
	if err = os.MkdirAll(cache, 0700); err != nil {
		logf("clipboard: %v", err)
		return func() {}
	}
	var listeners []net.Listener
	for _, port := range []int{clipPushPort, clipPullPort, transferPort} {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			for _, open := range listeners {
				open.Close()
			}
			logf("clipboard: port %d unavailable: %v", port, err)
			return func() {}
		}
		listeners = append(listeners, l)
	}
	if supported {
		setLinuxClipboardStatus(uiText("settings.clipboard.linux.synchronizes"), false)
	}
	if supported {
		c.sequence()
	}
	b := &clipBridge{authorizePeer: lifecycleConnectionFromQEMU, getHost: c.get, setHost: c.set, getPaths: c.getPaths, setPaths: c.setPaths,
		sequence: c.sequence, dropRequests: make(chan droppedFiles, 8),
		transferError: func(err error) {
			logf("file transfer: %v", err)
			showLinuxRuntimeError(uiText("error.linux.file_transfer_failed"), err.Error())
		},
		setDropPaths: c.setPaths, transfers: newFileTransferService(cache, clipboardTransferLimits), showTransfer: showLinuxTransfer}
	if !supported {
		// Explicit SDL drops still use the file transport on desktops that deny
		// background clipboard access. Never invoke wl-clipboard's surface fallback.
		b.getPaths, b.setPaths, b.setDropPaths, b.sequence = nil, nil, nil, nil
		b.setHost = func(clipItem) bool { return false }
	}
	desktopClipboard.Store(b)
	go b.acceptPush(listeners[0])
	go b.acceptPull(listeners[1])
	go b.transfers.Serve(listeners[2])
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(750 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case drop := <-b.dropRequests:
				if err := b.offerDroppedFiles(drop); err != nil {
					b.transferError(err)
				}
			case <-ticker.C:
				b.mu.Lock()
				conn, seq := b.pullConn, b.lastSequence
				b.mu.Unlock()
				if supported && conn != nil && c.sequence() != seq {
					b.sendCurrentHost(conn)
				}
			}
		}
	}()
	logf("clipboard: background sync=%t; explicit file drops enabled", supported)
	return func() {
		close(done)
		if c.portal != nil {
			c.portal.close()
		}
		c.owner.close()
		desktopClipboard.CompareAndSwap(b, nil)
		for _, l := range listeners {
			l.Close()
		}
		b.mu.Lock()
		if b.pullConn != nil {
			b.pullConn.Close()
			b.pullConn = nil
		}
		b.mu.Unlock()
	}
}

func showLinuxTransfer(p *transferProgress) {
	if !linuxGUIEnabled {
		return
	}
	w := startLinuxWindow(p.cancel)
	if w == nil {
		return
	}
	defer w.stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			w.update(linuxSetupState{Status: p.text.Load().(string)})
		}
	}
}
