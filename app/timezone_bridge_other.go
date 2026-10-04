//go:build !linux

package main

import "fmt"

// Only the Linux launcher in this repository follows the host zone live.
func timeZoneChardev(string) string {
	return fmt.Sprintf("socket,id=timezone0,host=127.0.0.1,port=%d,reconnect-ms=1000", timeZoneBridgePort)
}
