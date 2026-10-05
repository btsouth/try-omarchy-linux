//go:build linux

package main

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

var linuxVenusEnabled = true
var linuxHonorGuestPAT bool
var linuxGuestScale string

func parseLinuxScale(value string) (string, error) {
	if value == "auto" || value == "keep" {
		return value, nil
	}
	scale, err := strconv.ParseFloat(value, 64)
	if err != nil || !(scale >= 1 && scale <= 4) {
		return "", uiError(uiText("error.linux.scale_must_be_auto_keep_or_a_number"), nil)
	}
	return strconv.FormatFloat(scale, 'f', -1, 64), nil
}

func kernelAtLeast(release string, major, minor int) bool {
	parts := strings.Split(strings.TrimSpace(release), ".")
	if len(parts) < 2 {
		return false
	}
	ma, e1 := strconv.Atoi(parts[0])
	mi, e2 := strconv.Atoi(parts[1])
	return e1 == nil && e2 == nil && (ma > major || ma == major && mi >= minor)
}

// Older KVM kernels can stop the VM on CPU access to GPU buffer mappings.
// 6.16 also supplies guest PAT support needed by current Intel/xe and dGPUs.
// Keep hardware OpenGL on older hosts; an explicit override is available for
// distributions carrying the relevant KVM fixes as backports.
func linuxVenusPolicy(mode, release string) (bool, bool, error) {
	modern := kernelAtLeast(release, 6, 16)
	switch mode {
	case "auto":
		return modern, modern, nil
	case "on":
		return true, modern, nil
	case "off":
		return false, false, nil
	default:
		return false, false, uiError(uiText("error.linux.venus_must_be_auto_on_or_off"), nil)
	}
}

func configureLinuxGraphics(mode string) error {
	release, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	var err error
	linuxVenusEnabled, linuxHonorGuestPAT, err = linuxVenusPolicy(mode, string(release))
	if err == nil && !linuxVenusEnabled {
		logf("graphics: hardware OpenGL with software Vulkan; Venus requires a compatible KVM kernel (automatic minimum 6.16, host %s)", strings.TrimSpace(string(release)))
	}
	return err
}

func linuxGraphicsArgs(args []string, venus, honorPAT bool) []string {
	for i := 1; i < len(args); i++ {
		if args[i-1] == "-machine" && venus && honorPAT {
			args[i] = strings.ReplaceAll(args[i], ",accel=kvm", "")
		}
		if args[i-1] != "-device" || venus {
			continue
		}
		if strings.HasPrefix(args[i], "virtio-vga-gl,") {
			parts := strings.Split(args[i], ",")
			keep := parts[:1]
			for _, p := range parts[1:] {
				if !strings.HasPrefix(p, "blob=") && !strings.HasPrefix(p, "hostmem=") && !strings.HasPrefix(p, "venus=") {
					keep = append(keep, p)
				}
			}
			args[i] = strings.Join(keep, ",")
		} else if strings.HasPrefix(args[i], "{") {
			var device map[string]any
			if json.Unmarshal([]byte(args[i]), &device) == nil && device["driver"] == "virtio-vga-gl" {
				delete(device, "blob")
				delete(device, "hostmem")
				delete(device, "venus")
				data, _ := json.Marshal(device)
				args[i] = string(data)
			}
		}
	}
	if venus && honorPAT {
		args = append(args, "-accel", "kvm,honor-guest-pat=on")
	}
	return args
}
