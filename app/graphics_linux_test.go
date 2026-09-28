//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestLinuxVenusKernelPolicy(t *testing.T) {
	for _, tt := range []struct {
		mode, release string
		venus, pat    bool
	}{
		{"auto", "6.8.0-139-generic", false, false}, {"auto", "6.15.9", false, false},
		{"auto", "6.16.0", true, true}, {"auto", "7.2.6-arch2-1", true, true},
		{"auto", "unknown", false, false}, {"on", "6.8.0", true, false}, {"off", "7.2.0", false, false},
	} {
		v, p, e := linuxVenusPolicy(tt.mode, tt.release)
		if e != nil || v != tt.venus || p != tt.pat {
			t.Errorf("%+v: %v %v %v", tt, v, p, e)
		}
	}
	if _, _, e := linuxVenusPolicy("maybe", "7.0.0"); e == nil {
		t.Fatal("invalid mode accepted")
	}
}
func TestLinuxGraphicsKeepsHardwareOpenGL(t *testing.T) {
	args := []string{"-machine", "q35,accel=kvm", "-device", "virtio-vga-gl,blob=on,hostmem=2147483648,venus=on", "-device", "virtio-balloon-pci,free-page-reporting=on"}
	got := linuxGraphicsArgs(args, false, false)
	if got[3] != "virtio-vga-gl" || got[5] != args[5] {
		t.Fatal(got)
	}
	got = linuxGraphicsArgs([]string{"-machine", "q35,accel=kvm"}, true, true)
	if strings.Join(got, " ") != "-machine q35 -accel kvm,honor-guest-pat=on" {
		t.Fatal(got)
	}
}
