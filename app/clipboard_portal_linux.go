//go:build linux

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const linuxPortalDesktop = "org.freedesktop.portal.Desktop"
const linuxPortalObject = dbus.ObjectPath("/org/freedesktop/portal/desktop")

type linuxPortalClipboard struct {
	conn        *dbus.Conn
	session     dbus.ObjectPath
	signals     chan *dbus.Signal
	closed      chan struct{}
	mu          sync.Mutex
	mime        string
	data        []byte
	transferKey string
	addFiles    func(context.Context, string, []dbus.UnixFD) error
}

func linuxGNOMEWayland() bool {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		return false
	}
	for _, name := range strings.Split(strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")), ":") {
		if name == "GNOME" {
			return true
		}
	}
	return false
}

func linuxClipboardRestoreTokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "try-omarchy", "clipboard-restore-token"), nil
}

func linuxClipboardRestoreToken() string {
	path, err := linuxClipboardRestoreTokenPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 4096 {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func saveLinuxClipboardRestoreToken(token string) error {
	if len(token) == 0 || len(token) > 4096 || strings.ContainsRune(token, 0) {
		return errors.New("invalid clipboard permission token")
	}
	path, err := linuxClipboardRestoreTokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".clipboard-token-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.WriteString(token + "\n"); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// errLinuxClipboardDenied is the person's own "no", in GNOME's dialog or by
// leaving Share off. A timeout or a portal failure is not this error.
var errLinuxClipboardDenied = errors.New("desktop did not grant clipboard access")

func linuxPortalRequest(ctx context.Context, conn *dbus.Conn, signals <-chan *dbus.Signal, method string, args ...any) (map[string]dbus.Variant, error) {
	var path dbus.ObjectPath
	if err := conn.Object(linuxPortalDesktop, linuxPortalObject).CallWithContext(ctx, method, 0, args...).Store(&path); err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			_ = conn.Object(linuxPortalDesktop, path).Call("org.freedesktop.portal.Request.Close", 0).Err
			return nil, ctx.Err()
		case signal, ok := <-signals:
			if !ok {
				return nil, errors.New("portal connection closed")
			}
			if signal == nil || signal.Path != path || signal.Name != "org.freedesktop.portal.Request.Response" || len(signal.Body) != 2 {
				continue
			}
			code, valid := signal.Body[0].(uint32)
			results, resultsOK := signal.Body[1].(map[string]dbus.Variant)
			if !valid || !resultsOK {
				return nil, errors.New("invalid portal response")
			}
			if code != 0 {
				return nil, fmt.Errorf("%w (the request was declined)", errLinuxClipboardDenied)
			}
			return results, nil
		}
	}
}

func linuxPortalClipboardSession(c *linuxClipboard) (*linuxPortalClipboard, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			conn.Close()
		}
	}()
	signals := make(chan *dbus.Signal, 32)
	conn.Signal(signals)
	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stamp := fmt.Sprintf("tryomarchy_%d", time.Now().UnixNano())
	results, err := linuxPortalRequest(ctx, conn, signals, "org.freedesktop.portal.RemoteDesktop.CreateSession", map[string]dbus.Variant{"handle_token": dbus.MakeVariant(stamp + "_create"), "session_handle_token": dbus.MakeVariant(stamp + "_session")})
	if err != nil {
		return nil, err
	}
	value, ok := results["session_handle"]
	if !ok {
		return nil, errors.New("portal did not create a session")
	}
	sessionName, ok := value.Value().(string)
	session := dbus.ObjectPath(sessionName)
	if !ok || !session.IsValid() {
		return nil, errors.New("portal returned an invalid session")
	}
	defer func() {
		if closeOnError {
			_ = conn.Object(linuxPortalDesktop, session).Call("org.freedesktop.portal.Session.Close", 0).Err
		}
	}()
	// GNOME only enables its Share button when a device or a screen-cast
	// source is selected. Request pointer permission alone, never keyboard or
	// screen capture, and do not send any remote input events.
	deviceOptions := map[string]dbus.Variant{"handle_token": dbus.MakeVariant(stamp + "_devices"), "types": dbus.MakeVariant(uint32(2)), "persist_mode": dbus.MakeVariant(uint32(2))}
	if token := linuxClipboardRestoreToken(); token != "" {
		deviceOptions["restore_token"] = dbus.MakeVariant(token)
	}
	_, err = linuxPortalRequest(ctx, conn, signals, "org.freedesktop.portal.RemoteDesktop.SelectDevices", session, deviceOptions)
	if err != nil {
		return nil, err
	}
	if err = conn.Object(linuxPortalDesktop, linuxPortalObject).CallWithContext(ctx, "org.freedesktop.portal.Clipboard.RequestClipboard", 0, session, map[string]dbus.Variant{}).Err; err != nil {
		return nil, err
	}
	results, err = linuxPortalRequest(ctx, conn, signals, "org.freedesktop.portal.RemoteDesktop.Start", session, "", map[string]dbus.Variant{"handle_token": dbus.MakeVariant(stamp + "_start")})
	if err != nil {
		return nil, err
	}
	enabled, ok := results["clipboard_enabled"]
	if !ok || enabled.Value() != true {
		return nil, errLinuxClipboardDenied
	}
	if value, ok := results["restore_token"]; ok {
		if token, valid := value.Value().(string); valid && token != "" {
			if err := saveLinuxClipboardRestoreToken(token); err != nil {
				logf("clipboard permission token: %v", err)
			}
		}
	}
	if err = conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.portal.Clipboard")); err != nil {
		return nil, err
	}
	if err = conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.portal.Session"), dbus.WithMatchMember("Closed")); err != nil {
		return nil, err
	}
	p := &linuxPortalClipboard{conn: conn, session: session, signals: signals, closed: make(chan struct{})}
	closeOnError = false
	go p.run(c)
	return p, nil
}

func (p *linuxPortalClipboard) close() {
	select {
	case <-p.closed:
		return
	default:
		close(p.closed)
	}
	p.mu.Lock()
	key := p.transferKey
	p.transferKey = ""
	p.mu.Unlock()
	if key != "" {
		p.stopFileTransfer(key)
	}
	_ = p.conn.Object(linuxPortalDesktop, p.session).Call("org.freedesktop.portal.Session.Close", 0).Err
	p.conn.Close()
}

func (p *linuxPortalClipboard) selectMime(mimes []string) string {
	return linuxClipboardMIME(strings.Join(mimes, "\n"))
}

func linuxPortalMIMETypes(value any) []string {
	switch types := value.(type) {
	case []string:
		return types
	case []any:
		// GNOME 46 wraps the array in a single-element D-Bus struct.
		if len(types) == 1 {
			return linuxPortalMIMETypes(types[0])
		}
	}
	return nil
}

func (p *linuxPortalClipboard) read(mime string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var descriptor dbus.UnixFD
	if err := p.conn.Object(linuxPortalDesktop, linuxPortalObject).CallWithContext(ctx, "org.freedesktop.portal.Clipboard.SelectionRead", 0, p.session, mime).Store(&descriptor); err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(descriptor), "portal-clipboard")
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxClipboardImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxClipboardImageBytes {
		return nil, errors.New("portal clipboard exceeds size limit")
	}
	return data, nil
}

func (p *linuxPortalClipboard) run(c *linuxClipboard) {
	for {
		select {
		case <-p.closed:
			return
		case signal, ok := <-p.signals:
			if !ok || signal == nil {
				return
			}
			if signal.Name == "org.freedesktop.portal.Session.Closed" && signal.Path == p.session {
				select {
				case <-p.closed:
					return
				default:
				}
				setLinuxClipboardStatus("GNOME clipboard access ended. Restart Omarchy to request access again. File drops remain available.", true)
				c.mu.Lock()
				c.item, c.paths, c.key = clipItem{}, nil, [32]byte{}
				c.serial++
				c.mu.Unlock()
				return
			}
			if len(signal.Body) < 2 || signal.Body[0] != p.session {
				continue
			}
			switch signal.Name {
			case "org.freedesktop.portal.Clipboard.SelectionOwnerChanged":
				options, valid := signal.Body[1].(map[string]dbus.Variant)
				if !valid || options["session_is_owner"].Value() == true {
					continue
				}
				mimes := linuxPortalMIMETypes(options["mime_types"].Value())
				if len(mimes) == 0 {
					logf("portal clipboard: unsupported MIME list type %T", options["mime_types"].Value())
				}
				mime := p.selectMime(mimes)
				var data []byte
				if mime != "" {
					var err error
					data, err = p.read(mime)
					if err != nil {
						logf("portal clipboard read: %v", err)
						setLinuxClipboardStatus("Could not read the desktop clipboard: "+err.Error(), true)
						mime = ""
					}
				}
				var files []string
				if mime == "application/vnd.portal.filetransfer" {
					var grantErr error
					files, grantErr = linuxPortalClipboardFiles(string(data))
					if grantErr != nil {
						setLinuxClipboardStatus("Could not access files copied from the desktop: "+grantErr.Error(), true)
					}
				}
				c.mu.Lock()
				c.item, c.paths = clipItem{}, nil
				c.key = sha256.Sum256(append([]byte(mime+"\x00"), data...))
				c.serial++
				switch mime {
				case "application/vnd.portal.filetransfer":
					c.paths = files
				case "text/uri-list":
					c.paths, _ = linuxClipboardPaths(data)
				case "image/png":
					c.item = pngItem(data)
				case "text/plain;charset=utf-8", "text/plain", "UTF8_STRING":
					c.item = textItem(string(data))
				}
				c.mu.Unlock()
			case "org.freedesktop.portal.Clipboard.SelectionTransfer":
				if len(signal.Body) != 3 {
					continue
				}
				mime, _ := signal.Body[1].(string)
				serial, _ := signal.Body[2].(uint32)
				go p.write(mime, serial)
			}
		}
	}
}

func (p *linuxPortalClipboard) write(mime string, serial uint32) {
	p.mu.Lock()
	data := append([]byte(nil), p.data...)
	valid := p.mime == mime
	p.mu.Unlock()
	success := false
	if valid {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var descriptor dbus.UnixFD
		if err := p.conn.Object(linuxPortalDesktop, linuxPortalObject).CallWithContext(ctx, "org.freedesktop.portal.Clipboard.SelectionWrite", 0, p.session, serial).Store(&descriptor); err == nil {
			f := os.NewFile(uintptr(descriptor), "portal-clipboard-write")
			n, writeErr := f.Write(data)
			success = writeErr == nil && n == len(data) && f.Close() == nil
		}
	}
	_ = p.conn.Object(linuxPortalDesktop, linuxPortalObject).Call("org.freedesktop.portal.Clipboard.SelectionWriteDone", 0, p.session, serial, success).Err
}

func (p *linuxPortalClipboard) set(mime string, data []byte) bool {
	return p.setSelection(mime, data, "") == nil
}

func (p *linuxPortalClipboard) setSelection(mime string, data []byte, transferKey string) error {
	if len(data) > maxClipboardImageBytes {
		return errors.New("clipboard selection exceeds size limit")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	err := p.conn.Object(linuxPortalDesktop, linuxPortalObject).Call("org.freedesktop.portal.Clipboard.SetSelection", 0, p.session, map[string]dbus.Variant{"mime_types": dbus.MakeVariant([]string{mime})}).Err
	if err != nil {
		logf("portal clipboard write: %v", err)
		return err
	}
	previous := p.transferKey
	p.mime, p.data, p.transferKey = mime, append([]byte(nil), data...), transferKey
	if previous != "" && previous != transferKey {
		p.stopFileTransfer(previous)
	}
	return nil
}
