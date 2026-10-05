//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// sysfs metadata does not require opening or claiming a USB device. Interface
// entries lack busnum/devnum, and hubs (including root hubs) are not offered.
func readLinuxUSBDevices(ctx context.Context, root string) ([]usbDevice, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	devices := []usbDevice{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		dir := filepath.Join(root, entry.Name())
		read := func(name string) string {
			data, _ := os.ReadFile(filepath.Join(dir, name))
			return strings.TrimSpace(string(data))
		}
		number := func(name string, base int) (int, error) {
			value, err := strconv.ParseInt(read(name), base, 32)
			return int(value), err
		}
		bus, e1 := number("busnum", 10)
		addr, e2 := number("devnum", 10)
		vendor, e3 := number("idVendor", 16)
		product, e4 := number("idProduct", 16)
		class, e5 := number("bDeviceClass", 16)
		port := read("devpath")
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || class == 9 || port == "0" {
			continue
		}
		// Some composite hubs declare their class on an interface instead.
		hub := false
		interfaces, _ := filepath.Glob(filepath.Join(root, entry.Name()+":*", "bInterfaceClass"))
		for _, path := range interfaces {
			data, _ := os.ReadFile(path)
			value, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 16, 32)
			if value == 9 {
				hub = true
			}
		}
		if hub {
			continue
		}
		name := strings.Join(strings.Fields(read("manufacturer")+" "+read("product")), " ")
		if name == "" {
			name = fmt.Sprintf("USB %04x:%04x", vendor, product)
		}
		d := usbDevice{Bus: bus, Address: addr, Vendor: vendor, Product: product, Class: class, Port: port, Name: name, Connected: true}
		if bus == 0 || d.validate() != nil {
			continue
		}
		d.ID = d.identity()
		devices = append(devices, d)
	}
	return devices, nil
}

func linuxUSBNode(d usbDevice) string { return fmt.Sprintf("/dev/bus/usb/%03d/%03d", d.Bus, d.Address) }
func linuxUSBAccess(d usbDevice) error {
	// Check effective-user access including ACLs, without opening the device.
	return unix.Faccessat(unix.AT_FDCWD, linuxUSBNode(d), unix.R_OK|unix.W_OK, unix.AT_EACCESS)
}
func linuxUSBAccessError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if errors.Is(err, os.ErrPermission) || strings.Contains(message, "libusb_error_access") || strings.Contains(message, "permission denied") || strings.Contains(message, "access denied") {
		return uiError(uiText("usb.linux.inaccessible"), err)
	}
	return err
}

// The shared broker keeps exact port/address checks and release ownership.
// Only Linux enumeration and host access differ from Windows.
type linuxUSBQMP struct {
	usbQMP
	inventory func(context.Context) ([]usbDevice, error)
	access    func(usbDevice) error
}

func (q linuxUSBQMP) USBHostDevices(ctx context.Context) ([]usbDevice, error) {
	return q.inventory(ctx)
}
func (q linuxUSBQMP) Call(ctx context.Context, command string, args any, result any) error {
	if command == "device_add" {
		if a, ok := args.(map[string]any); ok && a["driver"] == "usb-host" {
			d := usbDevice{Bus: a["hostbus"].(int), Address: a["hostaddr"].(int)}
			if err := q.access(d); err != nil {
				return linuxUSBAccessError(err)
			}
		}
	}
	return linuxUSBAccessError(q.usbQMP.Call(ctx, command, args, result))
}
func newLinuxUSBBroker(q usbQMP) usbBroker {
	return usbBroker{linuxUSBQMP{q, func(ctx context.Context) ([]usbDevice, error) {
		return readLinuxUSBDevices(ctx, "/sys/bus/usb/devices")
	}, linuxUSBAccess}}
}
