//go:build linux

package main

import "testing"

func TestLinuxPortalMountAncestorIsExact(t *testing.T) {
	for _, tt := range []struct {
		path, target string
		want         bool
	}{
		{"/run/user/1001/doc", "../../flatpak/doc", true},
		{"/run/user/1001/omabox/private-runtime/doc", "../../../../flatpak/doc", true},
		{"/run/user/1001/doc", "../../home/fresh10i", false},
		{"/run/user/1001/doc/grant/vm", "../../flatpak/doc", false},
		{"/home/fresh10i/doc", "../../flatpak/doc", false},
		{"/run/user/1002/doc", "../../flatpak/doc", false},
	} {
		if got := linuxPortalMountAncestor(tt.path, tt.target, 1001); got != tt.want {
			t.Errorf("%s -> %s: %v, want %v", tt.path, tt.target, got, tt.want)
		}
	}
}
