//go:build linux

package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestLinuxKeyboardMapping(t *testing.T) {
	for _, tc := range []struct{ layout, variant, want string }{
		{"us,de", "intl,nodeadkeys", "us:intl"},
		{"us,de", ",nodeadkeys", "us"},
		{" de , us ", " nodeadkeys , intl ", "de:nodeadkeys"},
		{"us", "", "us"}, {"gb", "extd", "gb:extd"},
		{"", "intl", ""}, {"US", "", ""}, {"a", "", ""},
		{"toolonglayout", "", ""}, {"us", strings.Repeat("a", 33), ""},
		{"us;reboot", "", ""}, {"us", "intl:bad", ""},
	} {
		t.Run(tc.layout+"/"+tc.variant, func(t *testing.T) {
			layout, variant := linuxKeyboardEnvironment(tc.layout, tc.variant)
			if got := keyboardSpecForTest(layout, variant); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if layout != "" && hostLocaleCmdline("", layout, variant, "") != " tryomarchy.kb="+tc.want {
				t.Fatal("mapped value does not survive the guest command line")
			}
		})
	}
}

func keyboardSpecForTest(layout, variant string) string {
	if variant != "" {
		return layout + ":" + variant
	}
	return layout
}

func TestGNOMEKeyboardCurrentAndFallback(t *testing.T) {
	xkb := func(id string) linuxInputSource { return linuxInputSource{"xkb", id} }
	for _, tc := range []struct {
		name         string
		sources, mru []linuxInputSource
		want         string
	}{
		{"current", []linuxInputSource{xkb("us"), xkb("de+nodeadkeys")}, []linuxInputSource{xkb("de+nodeadkeys"), xkb("us")}, "de:nodeadkeys"},
		{"first", []linuxInputSource{xkb("gb+extd"), xkb("us")}, nil, "gb:extd"},
		{"stale mru", []linuxInputSource{xkb("fr")}, []linuxInputSource{xkb("us")}, "fr"},
		{"input method", []linuxInputSource{{"ibus", "pinyin"}, xkb("us+intl")}, []linuxInputSource{{"ibus", "pinyin"}}, "us:intl"},
		{"unknown method", []linuxInputSource{{"ibus", "pinyin"}}, nil, ""},
		{"invalid", []linuxInputSource{xkb("us+bad+variant"), xkb("us,de"), xkb("es")}, nil, "es"},
		{"empty", nil, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout, variant := gnomeKeyboard(tc.sources, tc.mru)
			if got := keyboardSpecForTest(layout, variant); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestKDEKeyboardFile(t *testing.T) {
	for _, tc := range []struct{ name, content, want string }{
		{"lists", "[Layout]\nUse=true\nLayoutList=de,us\nVariantList=nodeadkeys,intl\n", "de:nodeadkeys"},
		{"empty first variant", "[Layout]\nUse=true\nLayoutList=us,de\nVariantList=,nodeadkeys\n", "us"},
		{"groups", "[Other]\nUse=true\nLayoutList=fr\n[Layout]\nUse=true\n # comment\n LayoutList = gb\n VariantList = extd\n[Other]\nVariantList=intl\n", "gb:extd"},
		{"disabled", "[Layout]\nUse=false\nLayoutList=de\n", ""},
		{"empty variant", "[Layout]\nUse=true\nLayoutList=us\nVariantList=\\0\n", "us"},
		{"invalid", "[Layout]\nUse=true\nLayoutList=us;reboot\n", ""},
		{"default disabled", "[Layout]\nLayoutList=de\n", ""},
		{"missing layout", "[Layout]\nVariantList=intl\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", configDir)
			if err := os.WriteFile(filepath.Join(configDir, "kxkbrc"), []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			layout, variant := linuxKDEKeyboard()
			if got := keyboardSpecForTest(layout, variant); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	if layout, _ := readKDEKeyboard(filepath.Join(t.TempDir(), "missing")); layout != "" {
		t.Fatal("missing file yielded a layout")
	}
}

type testKeyboardPortal struct {
	values map[string]map[string]dbus.Variant
	err    *dbus.Error
}

func (p testKeyboardPortal) ReadAll(namespaces []string) (map[string]map[string]dbus.Variant, *dbus.Error) {
	if len(namespaces) != 1 || namespaces[0] != linuxInputSourcesNamespace {
		return nil, dbus.MakeFailedError(os.ErrInvalid)
	}
	return p.values, p.err
}

type testKeyboardLocaled struct {
	values map[string]dbus.Variant
	err    *dbus.Error
}

func (p testKeyboardLocaled) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	if iface != "org.freedesktop.locale1" {
		return nil, dbus.MakeFailedError(os.ErrInvalid)
	}
	return p.values, p.err
}

// No service directories: an unavailable source must not activate a desktop
// portal/backend on the developer's machine while exercising fallback.
func privateKeyboardBus(t *testing.T) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon unavailable")
	}
	config := filepath.Join(t.TempDir(), "bus.conf")
	data := `<busconfig><type>session</type><listen>unix:tmpdir=/tmp</listen><auth>EXTERNAL</auth><policy context="default"><allow own="*"/><allow send_destination="*"/><allow receive_sender="*"/></policy></busconfig>`
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(daemon, "--nofork", "--print-address=1", "--config-file="+config)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	address := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(out).ReadString('\n'); address <- strings.TrimSpace(line) }()
	select {
	case line := <-address:
		if line == "" {
			t.Fatal("private bus did not start")
		}
		return line
	case <-time.After(3 * time.Second):
		t.Fatal("private bus startup timed out")
		return ""
	}
}

func keyboardTestService(t *testing.T, address, name string, path dbus.ObjectPath, iface string, service any) *dbus.Conn {
	t.Helper()
	server, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	if err := server.Export(service, path, iface); err != nil {
		t.Fatal(err)
	}
	if reply, err := server.RequestName(name, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("service name: %v %v", reply, err)
	}
	return server
}

func TestLinuxKeyboardPrivateBusesAndPrecedence(t *testing.T) {
	// Both addresses point to a test-owned daemon, never the desktop bus.
	address := privateKeyboardBus(t)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", address)
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", address)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XKB_DEFAULT_LAYOUT", "es,us")
	t.Setenv("XKB_DEFAULT_VARIANT", "nodeadkeys,")
	sources := []linuxInputSource{{"xkb", "us"}, {"xkb", "de+nodeadkeys"}}
	portal := keyboardTestService(t, address, linuxPortalDesktop, linuxPortalObject, "org.freedesktop.portal.Settings", testKeyboardPortal{values: map[string]map[string]dbus.Variant{
		linuxInputSourcesNamespace: {"sources": dbus.MakeVariant(sources), "mru-sources": dbus.MakeVariant([]linuxInputSource{sources[1]})},
	}})
	localed := keyboardTestService(t, address, "org.freedesktop.locale1", "/org/freedesktop/locale1", "org.freedesktop.DBus.Properties", testKeyboardLocaled{values: map[string]dbus.Variant{
		"X11Layout": dbus.MakeVariant("gb,us"), "X11Variant": dbus.MakeVariant("extd,intl"),
	}})
	configPath := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "kxkbrc")
	if err := os.WriteFile(configPath, []byte("[Layout]\nUse=true\nLayoutList=fr\n"), 0600); err != nil {
		t.Fatal(err)
	}
	check := func(fn func() (string, string), want string) {
		t.Helper()
		layout, variant := fn()
		if got := keyboardSpecForTest(layout, variant); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
	check(linuxPortalKeyboard, "de:nodeadkeys")
	check(linuxLocaledKeyboard, "gb:extd")
	check(linuxHostKeyboard, "de:nodeadkeys")
	client, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	// A canceled call must not hang detection.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	layout, _ := readPortalKeyboard(ctx, client.Object(linuxPortalDesktop, linuxPortalObject))
	if layout != "" {
		t.Fatal("canceled request produced a layout")
	}
	// Remove the portal interface, then KDE, then localed, exercising the
	// production source order and permission/service failures on the wire.
	if _, err := portal.ReleaseName(linuxPortalDesktop); err != nil {
		t.Fatal(err)
	}
	check(linuxHostKeyboard, "fr")
	if err := os.Remove(configPath); err != nil {
		t.Fatal(err)
	}
	check(linuxHostKeyboard, "gb:extd")
	if _, err := localed.ReleaseName("org.freedesktop.locale1"); err != nil {
		t.Fatal(err)
	}
	check(linuxHostKeyboard, "es:nodeadkeys")
	var variant string
	_, layout, variant, _ = hostLocale("keep", "", "keep")
	if layout != "es" || variant != "nodeadkeys" {
		t.Fatal("follow host did not use detection")
	}
	_, layout, variant, _ = hostLocale("keep", "keep", "keep")
	if layout != "" || variant != "" {
		t.Fatal("keep guest used detection")
	}
	t.Setenv("XKB_DEFAULT_LAYOUT", "")
	check(linuxHostKeyboard, "")
}

func TestLinuxKeyboardPrecedenceAndUnknown(t *testing.T) {
	for winner := 0; winner < 5; winner++ {
		calls := 0
		var sources []func() (string, string)
		for i := 0; i < 4; i++ {
			sources = append(sources, func() (string, string) {
				calls++
				if i == winner {
					return "de", "nodeadkeys"
				}
				return "", ""
			})
		}
		layout, variant := detectLinuxKeyboard(sources...)
		if winner < 4 && (layout != "de" || variant != "nodeadkeys" || calls != winner+1) {
			t.Fatalf("winner %d: %q %q, %d calls", winner, layout, variant, calls)
		}
		if winner == 4 && (layout != "" || variant != "" || calls != 4) {
			t.Fatal("unavailable sources produced a layout")
		}
	}
	layout, _ := detectLinuxKeyboard(func() (string, string) { return "invalid layout", "" }, func() (string, string) { return "us", "" })
	if layout != "us" {
		t.Fatal("invalid source prevented fallback")
	}
}

func TestLinuxKeyboardBusErrorsAndMalformedValues(t *testing.T) {
	for _, tc := range []struct {
		name    string
		portal  testKeyboardPortal
		localed testKeyboardLocaled
	}{
		{"missing namespace", testKeyboardPortal{values: map[string]map[string]dbus.Variant{}}, testKeyboardLocaled{values: map[string]dbus.Variant{}}},
		{"wrong types", testKeyboardPortal{values: map[string]map[string]dbus.Variant{linuxInputSourcesNamespace: {"sources": dbus.MakeVariant("us"), "mru-sources": dbus.MakeVariant(uint32(0))}}}, testKeyboardLocaled{values: map[string]dbus.Variant{"X11Layout": dbus.MakeVariant(uint32(1)), "X11Variant": dbus.MakeVariant("")}}},
		{"denied", testKeyboardPortal{err: dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", nil)}, testKeyboardLocaled{err: dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", nil)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			address := privateKeyboardBus(t)
			keyboardTestService(t, address, linuxPortalDesktop, linuxPortalObject, "org.freedesktop.portal.Settings", tc.portal)
			keyboardTestService(t, address, "org.freedesktop.locale1", "/org/freedesktop/locale1", "org.freedesktop.DBus.Properties", tc.localed)
			client, err := dbus.Connect(address)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if layout, _ := readPortalKeyboard(ctx, client.Object(linuxPortalDesktop, linuxPortalObject)); layout != "" {
				t.Fatal("bad portal produced a layout")
			}
			if layout, _ := readLocaledKeyboard(ctx, client.Object("org.freedesktop.locale1", "/org/freedesktop/locale1")); layout != "" {
				t.Fatal("bad localed produced a layout")
			}
		})
	}
}
