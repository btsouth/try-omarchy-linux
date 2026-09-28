//go:build !windows && !linux

package main

func hostBatteryLine() (string, error) { return "", nil }
