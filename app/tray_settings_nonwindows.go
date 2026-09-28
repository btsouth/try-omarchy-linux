//go:build !windows && !linux

package main

func requestTraySettings() bool { return false }
