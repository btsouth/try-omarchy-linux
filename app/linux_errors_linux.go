//go:build linux

package main

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
)

func linuxSetupFailureHelp(err error) string {
	if errors.Is(err, errInsufficientDiskSpace) || isDiskFull(err) {
		return "Free space in the selected storage folder, then launch again. Your existing virtual machine was kept."
	}
	if errors.Is(err, os.ErrPermission) {
		return "Check that Try Omarchy can access the selected storage or shared folder, then try again."
	}
	if errors.Is(err, os.ErrNotExist) {
		return "Reconnect the drive or choose an available folder, then try again."
	}
	var urlErr *url.Error
	var netErr *net.OpError
	var dnsErr *net.DNSError
	if errors.As(err, &urlErr) || errors.As(err, &netErr) || errors.As(err, &dnsErr) {
		return "The Omarchy image source could not be reached. Check the connection and try again."
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "checksum mismatch") || strings.Contains(message, "authentication failed") {
		return "The image failed verification. Leave it unused and retry the download from a trusted source."
	}
	if strings.Contains(message, "http 404") {
		return "The configured Omarchy image is unavailable. Check the image source before trying again."
	}
	return "Try again. If the problem continues, keep the data folder and review its diagnostics."
}
