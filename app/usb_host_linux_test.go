//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/godbus/dbus/v5"
)

func usbSysfsFixture(t *testing.T, root, entry string, fields map[string]string) {
	t.Helper()
	dir := filepath.Join(root, entry)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for key, value := range fields {
		if err := os.WriteFile(filepath.Join(dir, key), []byte(value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestLinuxUSBSysfsListingAndPortIdentity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fields := map[string]string{"busnum": "1", "devnum": "3", "devpath": "2.4", "idVendor": "1234", "idProduct": "5678", "bDeviceClass": "00", "manufacturer": "Acme", "product": "Sample\n Device"}
	usbSysfsFixture(t, root, "1-2.4", fields)
	usbSysfsFixture(t, root, "usb1", map[string]string{"busnum": "1", "devnum": "1", "devpath": "0", "idVendor": "1d6b", "idProduct": "0003", "bDeviceClass": "09"})
	fields["bDeviceClass"] = "09"
	usbSysfsFixture(t, root, "1-2", fields)
	fields["bDeviceClass"] = "00"
	usbSysfsFixture(t, root, "1-3", fields)
	usbSysfsFixture(t, root, "1-3:1.0", map[string]string{"bInterfaceClass": "09"})
	usbSysfsFixture(t, root, "1-2.4:1.0", map[string]string{"bInterfaceClass": "03"})
	usbSysfsFixture(t, root, "gone", map[string]string{"idVendor": "bad"})
	// Real sysfs contains symlinks, not only directories.
	target := t.TempDir()
	fields["busnum"], fields["devpath"], fields["manufacturer"], fields["product"] = "2", "1", "", ""
	usbSysfsFixture(t, target, "device", fields)
	if err := os.Symlink(filepath.Join(target, "device"), filepath.Join(root, "2-1")); err != nil {
		t.Fatal(err)
	}
	devices, err := readLinuxUSBDevices(ctx, root)
	if err != nil || len(devices) != 2 {
		t.Fatal(devices, err)
	}
	d := devices[0]
	if d.Name != "Acme Sample Device" || d.Bus != 1 || d.Port != "2.4" || d.Address != 3 || d.Vendor != 0x1234 || d.Product != 0x5678 || !d.Connected {
		t.Fatal(d)
	}
	if devices[1].Name != "USB 1234:5678" {
		t.Fatal(devices[1])
	}
	if err := os.WriteFile(filepath.Join(root, "1-2.4", "devnum"), []byte("8"), 0600); err != nil {
		t.Fatal(err)
	}
	next, _ := readLinuxUSBDevices(ctx, root)
	if next[0].ID != d.ID || next[0].Address != 8 {
		t.Fatal(next)
	}
	if err := os.WriteFile(filepath.Join(root, "1-2.4", "devpath"), []byte("2.5"), 0600); err != nil {
		t.Fatal(err)
	}
	next, _ = readLinuxUSBDevices(ctx, root)
	if next[0].ID == d.ID {
		t.Fatal("moving port retained identity")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := readLinuxUSBDevices(cancelled, root); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := readLinuxUSBDevices(ctx, filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing sysfs reported empty success")
	}
}

func linuxUSBFakeBroker(f *usbFake, devices []usbDevice, access func(usbDevice) error) usbBroker {
	return usbBroker{linuxUSBQMP{f, func(context.Context) ([]usbDevice, error) { return devices, nil }, access}}
}
func TestLinuxUSBAttachReleaseAndPermissionDenied(t *testing.T) {
	devices, _ := parseUSBHostDevices(usbSample)
	d := devices[0]
	for _, denied := range []bool{false, true} {
		t.Run(fmt.Sprint(denied), func(t *testing.T) {
			f := &usbFake{objects: []usbQOMEntry{{Name: usbControllerID, Type: "child<qemu-xhci>"}}}
			checks := 0
			broker := linuxUSBFakeBroker(f, devices, func(current usbDevice) error {
				checks++
				if linuxUSBNode(current) != "/dev/bus/usb/001/003" {
					t.Fatal(current)
				}
				if denied {
					return &os.PathError{Op: "access", Path: linuxUSBNode(current), Err: syscall.EACCES}
				}
				return nil
			})
			err := broker.Attach(context.Background(), d)
			if checks != 1 {
				t.Fatalf("access checks=%d", checks)
			}
			for _, call := range f.calls {
				if call == "human-monitor-command" {
					t.Fatal("Linux queried libusb for inventory")
				}
			}
			if denied {
				if !errors.Is(err, syscall.EACCES) || !strings.Contains(err.Error(), "not accessible to your user account") || f.added != nil {
					t.Fatal(err, f.added)
				}
			} else {
				if err != nil || f.added["hostport"] != "2.4" || f.added["auto-reconnect"] != false {
					t.Fatal(err, f.added)
				}
				if err := broker.Detach(context.Background(), d.ID); err != nil || len(f.objects) != 1 {
					t.Fatal(err, f.objects)
				}
			}
		})
	}
	for _, err := range []error{syscall.EACCES, errors.New("LIBUSB_ERROR_ACCESS"), errors.New("Permission denied"), errors.New("Access denied")} {
		if got := linuxUSBAccessError(err); !strings.Contains(got.Error(), "not accessible") || !errors.Is(got, err) {
			t.Fatal(got)
		}
	}
	busy := errors.New("device busy")
	if linuxUSBAccessError(busy) != busy {
		t.Fatal("busy misreported as access denied")
	}
}

func TestLinuxUSBBootGrantIsOptionalAndAttemptedOnce(t *testing.T) {
	devices, _ := parseUSBHostDevices(usbSample)
	for _, mode := range []string{"disabled", "missing", "busy", "permission", "unsupported", "changed-address", "changed-port"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			pref := usbPreferences{Enabled: mode != "disabled", Device: selectionForUSB(devices[0])}
			if err := saveUSBPreferences(dir, pref); err != nil {
				t.Fatal(err)
			}
			live := append([]usbDevice(nil), devices...)
			if mode == "missing" {
				live = nil
			}
			if mode == "changed-address" {
				live[0].Address = 8
			}
			if mode == "changed-port" {
				live[0].Port = "3"
				live[0].ID = live[0].identity()
			}
			f := &usbFake{objects: []usbQOMEntry{{Name: usbControllerID, Type: "child<qemu-xhci>"}}, failAttach: mode == "busy", oldRuntime: mode == "unsupported"}
			broker := linuxUSBFakeBroker(f, live, func(usbDevice) error {
				if mode == "permission" {
					return syscall.EACCES
				}
				return nil
			})
			connects, closes := 0, 0
			err := applyLinuxBootUSB(context.Background(), dir, func(context.Context) (usbBroker, func(), error) { connects++; return broker, func() { closes++ }, nil })
			if mode == "disabled" {
				if err != nil || connects != 0 {
					t.Fatal(err, connects)
				}
				return
			}
			if connects != 1 || closes != 1 {
				t.Fatal(connects, closes)
			}
			adds := 0
			for _, call := range f.calls {
				if call == "device_add" {
					adds++
				}
			}
			if adds > 1 {
				t.Fatal("retried boot attach", f.calls)
			}
			if mode == "changed-address" {
				if err != nil || f.added["hostaddr"] != 8 {
					t.Fatal(err, f.added)
				}
			} else if err == nil {
				t.Fatal("unavailable grant succeeded")
			}
		})
	}
}

func TestLinuxUSBTrayAndDisconnectedSavedChoice(t *testing.T) {
	drain := func(ch chan struct{}) {
		for len(ch) > 0 {
			<-ch
		}
	}
	drain(linuxUSBRequests)
	defer drain(linuxUSBRequests)
	menu := &linuxTrayMenu{}
	menu.Event(linuxTrayUSB, "hovered", dbus.MakeVariant(0), 0)
	if len(linuxUSBRequests) != 0 {
		t.Fatal("hover queued USB")
	}
	menu.Event(linuxTrayUSB, "clicked", dbus.MakeVariant(0), 0)
	menu.Event(linuxTrayUSB, "clicked", dbus.MakeVariant(0), 0)
	if len(linuxUSBRequests) != 1 {
		t.Fatal("USB requests were not coalesced")
	}
	devices, _ := parseUSBHostDevices(usbSample)
	pref := usbPreferences{Enabled: true, Device: selectionForUSB(devices[0])}
	choices := usbSelectionChoices(nil, pref.Device)
	state := linuxUSBState(choices, true, pref, "")
	if len(state.Sections[0].Rows) != 1 || !strings.Contains(state.Sections[0].Rows[0].Detail, "Not connected") || state.Sections[0].Rows[0].Reply != "device:0" {
		t.Fatal(state)
	}
	dir := t.TempDir()
	if err := saveUSBPreferences(dir, pref); err != nil {
		t.Fatal(err)
	}
	pref.Enabled = false
	if err := saveLinuxUSBChoice(dir, pref); err != nil {
		t.Fatal(err)
	}
	saved, err := loadUSBPreferences(dir)
	if err != nil || saved.Enabled || saved.Device == nil || *saved.Device != *pref.Device {
		t.Fatal(saved, err)
	}
}

// QOM retains a disconnected claim until Release. A device re-enumerated at
// that port must not be reported as reclaimed by the existing attachment.
type linuxUSBClaimFake struct {
	usbFake
	device   usbDevice
	attached bool
}

func (f *linuxUSBClaimFake) Call(ctx context.Context, command string, args any, result any) error {
	if command != "qom-get" {
		return f.usbFake.Call(ctx, command, args, result)
	}
	property := args.(map[string]any)["property"]
	switch property {
	case "hostbus":
		*result.(*int) = f.device.Bus
	case "hostaddr":
		*result.(*int) = f.device.Address
	case "vendorid":
		*result.(*int) = f.device.Vendor
	case "productid":
		*result.(*int) = f.device.Product
	case "hostport":
		*result.(*string) = f.device.Port
	case "attached":
		*result.(*bool) = f.attached
	default:
		return fmt.Errorf("unexpected property: %s", property)
	}
	return nil
}
func TestLinuxUSBDisconnectedClaimCanBeReleasedWithoutReclaiming(t *testing.T) {
	devices, _ := parseUSBHostDevices(usbSample)
	d := devices[0]
	for _, present := range []bool{false, true} {
		f := &linuxUSBClaimFake{usbFake: usbFake{objects: []usbQOMEntry{{Name: usbControllerID, Type: "child<qemu-xhci>"}, {Name: d.ID, Type: "child<usb-host>"}}}, device: d, attached: present}
		var live []usbDevice
		if present {
			replacement := d
			replacement.Address = 8
			live = []usbDevice{replacement}
		}
		broker := usbBroker{linuxUSBQMP{f, func(context.Context) ([]usbDevice, error) { return live, nil }, func(usbDevice) error { t.Fatal("release checked host access"); return nil }}}
		listed, err := broker.Devices(context.Background())
		if err != nil || len(listed) != 1 || !listed[0].Claimed || listed[0].Connected {
			t.Fatal(listed, err)
		}
		if err := broker.Attach(context.Background(), listed[0]); err == nil || f.added != nil {
			t.Fatal("reclaimed reconnected device", err)
		}
		if err := broker.Detach(context.Background(), d.ID); err != nil || len(f.objects) != 1 {
			t.Fatal(err, f.objects)
		}
	}
}
