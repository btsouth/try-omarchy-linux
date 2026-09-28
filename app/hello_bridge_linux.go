//go:build linux

package main

import "strings"

// Windows Hello has no Linux bridge, so nothing listens on its port. QEMU
// aborts at startup when a reconnecting socket chardev's first connection is
// refused, so the Linux launcher leaves the authentication port out.
func linuxWithoutWindowsHello(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if i+1 < len(args) && ((args[i] == "-chardev" && strings.Contains(args[i+1], "id=hello0,")) ||
			(args[i] == "-device" && strings.Contains(args[i+1], "chardev=hello0,"))) {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}
