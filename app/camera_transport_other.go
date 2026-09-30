//go:build !linux

package main

import "fmt"

func cameraChardev(string) string {
	return fmt.Sprintf("socket,id=cam0,host=127.0.0.1,port=%d,reconnect-ms=1000", cameraPort)
}
