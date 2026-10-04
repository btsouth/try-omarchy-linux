//go:build !windows

package main

// activateRunningInstance has nothing to bring forward outside Windows, so a
// second launch reports that the lifecycle port is taken.
func activateRunningInstance() bool { return false }
