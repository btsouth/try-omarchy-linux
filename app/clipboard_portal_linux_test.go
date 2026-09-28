//go:build linux

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestLinuxGNOMEWaylandSelection(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	if !linuxGNOMEWayland() {
		t.Fatal("GNOME Wayland did not select the portal")
	}
	t.Setenv("WAYLAND_DISPLAY", "")
	if linuxGNOMEWayland() {
		t.Fatal("GNOME X11 selected the Wayland portal")
	}
}

type testClipboardPortal struct {
	conn          *dbus.Conn
	mu            sync.Mutex
	mime          string
	restored      string
	deny          bool
	denySelection bool
}

const testPortalSession = dbus.ObjectPath("/org/freedesktop/portal/desktop/session/test/clipboard")

func (p *testClipboardPortal) response(path dbus.ObjectPath, results map[string]dbus.Variant) {
	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = p.conn.Emit(path, "org.freedesktop.portal.Request.Response", uint32(0), results)
	}()
}

func (p *testClipboardPortal) CreateSession(options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	path := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/test/create")
	p.response(path, map[string]dbus.Variant{"session_handle": dbus.MakeVariant(string(testPortalSession))})
	return path, nil
}

func (p *testClipboardPortal) SelectDevices(session dbus.ObjectPath, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	if session != testPortalSession || options["types"].Value() != uint32(2) {
		return "", dbus.MakeFailedError(os.ErrInvalid)
	}
	if value, ok := options["restore_token"]; ok {
		p.mu.Lock()
		p.restored, _ = value.Value().(string)
		p.mu.Unlock()
	}
	path := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/test/devices")
	p.response(path, map[string]dbus.Variant{})
	return path, nil
}

func (p *testClipboardPortal) Start(session dbus.ObjectPath, parent string, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	path := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/test/start")
	p.mu.Lock()
	deny := p.deny
	p.mu.Unlock()
	if deny {
		p.response(path, map[string]dbus.Variant{"clipboard_enabled": dbus.MakeVariant(false)})
		return path, nil
	}
	p.response(path, map[string]dbus.Variant{"clipboard_enabled": dbus.MakeVariant(true), "restore_token": dbus.MakeVariant("private-consent-token")})
	return path, nil
}

func (p *testClipboardPortal) RequestClipboard(session dbus.ObjectPath, options map[string]dbus.Variant) *dbus.Error {
	if session != testPortalSession {
		return dbus.MakeFailedError(os.ErrInvalid)
	}
	return nil
}

func (p *testClipboardPortal) SetSelection(session dbus.ObjectPath, options map[string]dbus.Variant) *dbus.Error {
	if session != testPortalSession {
		return dbus.MakeFailedError(os.ErrInvalid)
	}
	mimes, _ := options["mime_types"].Value().([]string)
	if len(mimes) != 1 {
		return dbus.MakeFailedError(os.ErrInvalid)
	}
	p.mu.Lock()
	if p.denySelection {
		p.mu.Unlock()
		return dbus.MakeFailedError(errors.New("selection denied"))
	}
	p.mime = mimes[0]
	p.mu.Unlock()
	return nil
}

func (p *testClipboardPortal) SelectionRead(session dbus.ObjectPath, mime string) (dbus.UnixFD, *dbus.Error) {
	if session != testPortalSession || mime != "text/plain" {
		return 0, dbus.MakeFailedError(os.ErrInvalid)
	}
	read, write, err := os.Pipe()
	if err != nil {
		return 0, dbus.MakeFailedError(err)
	}
	go func() {
		_, _ = write.Write([]byte("host portal text"))
		_ = write.Close()
	}()
	return dbus.UnixFD(read.Fd()), nil
}

func (p *testClipboardPortal) Close() *dbus.Error { return nil }

type testFileTransferPortal struct {
	mu      sync.Mutex
	files   [][]byte
	keys    []string
	stopped []string
	denyAdd bool
	retrievePath string
	denyRetrieve bool
}

func (p *testFileTransferPortal) StartTransfer(options map[string]dbus.Variant) (string, *dbus.Error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := "test-file-transfer-" + string(rune('a'+len(p.keys)))
	p.keys = append(p.keys, key)
	return key, nil
}

func (p *testFileTransferPortal) addFiles(key string, fds []dbus.UnixFD) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.denyAdd {
		return errors.New("files denied")
	}
	if len(p.keys) == 0 || key != p.keys[len(p.keys)-1] {
		return errors.New("unknown transfer")
	}
	for _, fd := range fds {
		copyFD, err := syscall.Dup(int(fd))
		if err != nil {
			return err
		}
		file := os.NewFile(uintptr(copyFD), "test-portal-file")
		data, err := io.ReadAll(file)
		if err != nil {
			file.Close()
			return err
		}
		p.files = append(p.files, data)
		file.Close()
	}
	return nil
}

func (p *testFileTransferPortal) StopTransfer(key string) *dbus.Error {
	p.mu.Lock()
	p.stopped = append(p.stopped, key)
	p.mu.Unlock()
	return nil
}

func (p *testFileTransferPortal) RetrieveFiles(key string, options map[string]dbus.Variant) ([]string, *dbus.Error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.denyRetrieve || key != "host-file-key" {
		return nil, dbus.MakeFailedError(errors.New("file grant denied"))
	}
	return []string{p.retrievePath}, nil
}

func TestLinuxClipboardPortalPrivateBus(t *testing.T) {
	if os.Getenv("TRYOMARCHY_TEST_PRIVATE_BUS") != "1" || !strings.HasPrefix(os.Getenv("DBUS_SESSION_BUS_ADDRESS"), "unix:") {
		t.Skip("requires a test-owned dbus-run-session bus")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if reply, err := server.RequestName(linuxPortalDesktop, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("own private portal name: %v, %v", reply, err)
	}
	fake := &testClipboardPortal{conn: server}
	for _, iface := range []string{"org.freedesktop.portal.RemoteDesktop", "org.freedesktop.portal.Clipboard"} {
		if err := server.Export(fake, linuxPortalObject, iface); err != nil {
			t.Fatal(err)
		}
	}
	if err := server.Export(fake, testPortalSession, "org.freedesktop.portal.Session"); err != nil {
		t.Fatal(err)
	}
	files := &testFileTransferPortal{}
	if err := server.Export(files, linuxDocumentsObject, "org.freedesktop.portal.FileTransfer"); err != nil {
		t.Fatal(err)
	}
	if reply, err := server.RequestName(linuxDocumentsPortal, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("own private documents name: %v, %v", reply, err)
	}
	c := &linuxClipboard{}
	p, err := linuxPortalClipboardSession(c)
	if err != nil {
		t.Fatal(err)
	}
	c.portal = p
	p.addFiles = func(_ context.Context, key string, fds []dbus.UnixFD) error {
		return files.addFiles(key, fds)
	}
	defer p.close()
	if !c.set(textItem("portal round trip")) {
		t.Fatal("portal rejected guest text selection")
	}
	fake.mu.Lock()
	mime := fake.mime
	fake.mu.Unlock()
	if mime != "text/plain;charset=utf-8" {
		t.Fatalf("portal selection MIME: %q", mime)
	}
	file := filepath.Join(t.TempDir(), "guest 世界.txt")
	if err := os.WriteFile(file, []byte("private guest file"), 0600); err != nil {
		t.Fatal(err)
	}
	if !c.setPaths([]string{file}) {
		t.Fatalf("file transfer portal rejected guest file: %v", linuxClipboardStatus.Load())
	}
	files.mu.Lock()
	gotFiles := append([][]byte(nil), files.files...)
	key := files.keys[0]
	files.mu.Unlock()
	fake.mu.Lock()
	mime = fake.mime
	fake.mu.Unlock()
	if mime != "application/vnd.portal.filetransfer" || len(gotFiles) != 1 || string(gotFiles[0]) != "private guest file" {
		t.Fatalf("portal file grant: mime=%q files=%q", mime, gotFiles)
	}
	if !c.set(textItem("next selection")) {
		t.Fatal("text selection after file transfer failed")
	}
	files.mu.Lock()
	stopped := append([]string(nil), files.stopped...)
	files.mu.Unlock()
	if len(stopped) != 1 || stopped[0] != key {
		t.Fatalf("previous file transfer was not stopped: %q", stopped)
	}
	files.mu.Lock()
	files.denyAdd = true
	files.mu.Unlock()
	if c.setPaths([]string{file}) {
		t.Fatal("denied file grant was published")
	}
	files.mu.Lock()
	failedKey := files.keys[len(files.keys)-1]
	stopped = append([]string(nil), files.stopped...)
	files.mu.Unlock()
	if len(stopped) != 2 || stopped[1] != failedKey {
		t.Fatalf("failed grant was not cleaned up: %q", stopped)
	}
	files.mu.Lock()
	files.retrievePath = file
	files.mu.Unlock()
	gotPaths, err := linuxPortalClipboardFiles("host-file-key\n")
	if err != nil || len(gotPaths) != 1 || gotPaths[0] != file {
		t.Fatalf("file transfer portal did not grant host file: %q, %v", gotPaths, err)
	}
	files.mu.Lock()
	files.denyRetrieve = true
	files.mu.Unlock()
	if _, err := linuxPortalClipboardFiles("host-file-key"); err == nil || !strings.Contains(err.Error(), "grant failed") {
		t.Fatalf("denied file transfer did not fail clearly: %v", err)
	}
	// GNOME 46 emits the MIME array inside a one-field D-Bus struct.
	if err := server.Emit(linuxPortalObject, "org.freedesktop.portal.Clipboard.SelectionOwnerChanged", testPortalSession, map[string]dbus.Variant{"mime_types": dbus.MakeVariant(struct{ Types []string }{[]string{"text/plain"}}), "session_is_owner": dbus.MakeVariant(false)}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for c.sequence() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if c.sequence() == 0 {
		t.Fatal("portal selection notification did not reach the bridge")
	}
	item, ok := c.get()
	if !ok || item.Kind != clipText || string(item.Data) != "host portal text" {
		t.Fatalf("portal read did not reach the guest bridge: %+v, %v", item, ok)
	}
	if token := linuxClipboardRestoreToken(); token != "private-consent-token" {
		t.Fatalf("consent token was not saved: %q", token)
	}
	if err := server.Emit(testPortalSession, "org.freedesktop.portal.Session.Closed", map[string]dbus.Variant{}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for c.sequence() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if item, ok := c.get(); ok || item.Kind != "" {
		t.Fatalf("revoked portal selection remained available: %+v, %v", item, ok)
	}
	p.close()
	second, err := linuxPortalClipboardSession(&linuxClipboard{})
	if err != nil {
		t.Fatal(err)
	}
	defer second.close()
	fake.mu.Lock()
	restored := fake.restored
	fake.mu.Unlock()
	if restored != "private-consent-token" {
		t.Fatalf("saved consent token was not offered to portal: %q", restored)
	}
	fake.mu.Lock()
	fake.deny = true
	fake.mu.Unlock()
	if _, err := linuxPortalClipboardSession(&linuxClipboard{}); err == nil || !strings.Contains(err.Error(), "did not grant") {
		t.Fatalf("denied portal session did not fail: %v", err)
	}
}
