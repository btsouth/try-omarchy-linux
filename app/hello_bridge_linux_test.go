//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestLinuxQemuArgsOmitWindowsHelloPort(t *testing.T) {
	cfg := &config{disk: "/home/me/try-omarchy/vm/disk.raw", diskFormat: "raw"}
	args := linuxQemuArgs(cfg, []string{
		"-chardev", "socket,id=cam0,host=127.0.0.1,port=4453,reconnect-ms=1000",
		"-device", "virtserialport,chardev=cam0,name=dev.tryomarchy.camera",
		"-chardev", "socket,id=hello0,host=127.0.0.1,port=4455,reconnect-ms=1000",
		"-device", "virtserialport,chardev=hello0,name=dev.tryomarchy.authentication",
		"-device", "qemu-xhci,id=tryomarchy-usb",
	})
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "hello0") || strings.Contains(joined, "dev.tryomarchy.authentication") {
		t.Fatalf("Windows Hello port reached the Linux VM: %v", args)
	}
	if !strings.Contains(joined, "chardev=cam0,name=dev.tryomarchy.camera") || !strings.Contains(joined, "qemu-xhci,id=tryomarchy-usb") {
		t.Fatalf("unrelated devices were removed: %v", args)
	}
}
