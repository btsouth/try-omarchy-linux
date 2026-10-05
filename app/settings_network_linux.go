//go:build linux

package main

import (
	"fmt"
	"strconv"
	"strings"
)

func linuxNetworkForm(forwards []string) (sshEnabled bool, sshPort, additional string) {
	sshPort = "2222"
	var rest []string
	for _, value := range forwards {
		forward, err := parseForward(value)
		if err == nil && !sshEnabled && forward.proto == "tcp" && forward.guestPort == 22 && forward.address() == "127.0.0.1" {
			sshEnabled = true
			sshPort = strconv.Itoa(forward.hostPort)
			continue
		}
		rest = append(rest, value)
	}
	return sshEnabled, sshPort, strings.Join(rest, "\n")
}

func linuxForwardsFromForm(sshEnabled bool, sshPort, additional string) (string, error) {
	text := strings.TrimSpace(additional)
	if !sshEnabled {
		return text, nil
	}
	port, err := strconv.Atoi(strings.TrimSpace(sshPort))
	if err != nil || port < 1024 || port > 65535 {
		return "", uiError(uiText("settings.network.linux.choose_an_ssh_port_between_1024_and_65535"), nil)
	}
	if text != "" {
		text += "\n"
	}
	return text + fmt.Sprintf("tcp:%d:22", port), nil
}

func validateLinuxLocalForwards(values []string) error {
	for _, value := range values {
		forward, err := parseForward(value)
		if err != nil {
			return err
		}
		if forward.address() != "127.0.0.1" {
			return uiError(uiText("settings.network.linux.linux_port_forwarding_is_currently_available_only_on"), nil)
		}
	}
	return nil
}
