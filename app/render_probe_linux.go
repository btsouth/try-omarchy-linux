//go:build linux

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func configureLinuxRender(cfg *config, driver string) string {
	cfg.runtimeID = linuxRenderFileIdentity(cfg.qemu)
	cfg.displayDriver = driver
	probe, err := loadRenderProbe(cfg.dir)
	if err != nil {
		logf("could not read the rendering result: %v", err)
	}
	var reason string
	cfg.useGpu, reason = startWithGPU(cfg.renderMode, probe, cfg.runtimeID, cfg.displayDriver, time.Now())
	return reason
}

// Linux runs the installed executable, rather than a Windows runtime receipt.
// Include its resolved path so changing a user-managed runtime also retries.
func linuxRenderFileIdentity(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d", resolved, info.Size(), info.ModTime().UnixNano())
}

// Both the kernel driver and the userspace GL extension matter. File identities
// catch Mesa/NVIDIA upgrades inside Flatpak as well as native installs, without
// opening a display or starting a graphics probe on every launch.
func linuxDisplayDriverIdentity() string {
	patterns := []string{
		"/sys/class/drm/renderD*/device/vendor",
		"/sys/class/drm/renderD*/device/device",
		"/sys/class/drm/renderD*/device/driver/module/version",
		"/sys/class/drm/renderD*/device/driver/module/srcversion",
		"/usr/lib/dri/*_dri.so", "/usr/lib64/dri/*_dri.so",
		"/usr/lib/*-linux-gnu/dri/*_dri.so",
		"/usr/lib/*-linux-gnu/GL/*/lib/dri/*_dri.so",
		"/usr/lib/*-linux-gnu/GL/*/lib/libGLX*.so*",
		"/usr/lib/libGLX*.so*", "/usr/lib64/libGLX*.so*",
	}
	settings := fmt.Sprintf("venus=%t;pat=%t", linuxVenusEnabled, linuxHonorGuestPAT)
	for _, name := range []string{"DRI_PRIME", "__NV_PRIME_RENDER_OFFLOAD", "__GLX_VENDOR_LIBRARY_NAME", "MESA_LOADER_DRIVER_OVERRIDE", "LIBGL_ALWAYS_SOFTWARE"} {
		settings += ";" + name + "=" + os.Getenv(name)
	}
	return linuxGraphicsIdentity("/proc/sys/kernel/osrelease", patterns, settings)
}

func linuxGraphicsIdentity(kernel string, patterns []string, settings string) string {
	var entries []string
	if data, err := os.ReadFile(kernel); err == nil {
		entries = append(entries, "kernel="+strings.TrimSpace(string(data)))
	}
	for _, pattern := range patterns {
		paths, _ := filepath.Glob(pattern)
		for _, path := range paths {
			if strings.HasPrefix(path, "/sys/") {
				if data, err := os.ReadFile(path); err == nil {
					entries = append(entries, path+"="+strings.TrimSpace(string(data)))
				}
			} else if id := linuxRenderFileIdentity(path); id != "" {
				entries = append(entries, id)
			}
		}
	}
	sort.Strings(entries)
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(settings+"\n"+strings.Join(entries, "\n"))))
}
