//go:build linux

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func measureHostResources(sampleCPU bool) hostResources {
	h := hostResources{LogicalCPUs: runtime.NumCPU()}
	if sampleCPU {
		if busy0, total0, ok := readProcStat(); ok {
			time.Sleep(750 * time.Millisecond)
			if busy1, total1, ok := readProcStat(); ok && total1 > total0 {
				h.CPUBusy, h.CPUKnown = float64(busy1-busy0)/float64(total1-total0), true
			}
		}
	}
	if f, err := os.Open("/proc/meminfo"); err == nil {
		h.TotalMiB, h.AvailableMiB = parseMeminfo(f)
		f.Close()
	}
	return h
}

func readProcStat() (busy, total uint64, ok bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	return parseProcStatCPU(f)
}

// parseProcStatCPU reads the aggregate "cpu" line: user nice system idle
// iowait irq softirq steal. Idle and iowait count as not busy; the guest
// fields are already included in user and nice.
func parseProcStatCPU(r io.Reader) (busy, total uint64, ok bool) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && line == "" {
		return 0, 0, false
	}
	fields := strings.Fields(line)
	if len(fields) < 9 || fields[0] != "cpu" {
		return 0, 0, false
	}
	var values [8]uint64
	for i := range values {
		v, err := strconv.ParseUint(fields[i+1], 10, 64)
		if err != nil {
			return 0, 0, false
		}
		values[i] = v
		total += v
	}
	idle := values[3] + values[4]
	return total - idle, total, true
}

func parseMeminfo(r io.Reader) (totalMiB, availableMiB int) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		kib, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalMiB = kib / 1024
		case "MemAvailable:":
			availableMiB = kib / 1024
		}
	}
	return totalMiB, availableMiB
}

// hostLocale reports the host's time zone and language for the guest, with
// the same overrides as Windows: blank follows the host, "keep" leaves the
// guest alone. Compositors can expose their XKB layout through the standard
// XKB_DEFAULT_LAYOUT and XKB_DEFAULT_VARIANT environment variables.
func hostLocale(zoneOverride, keyboardOverride, localeOverride string) (zone, layout, variant, locale string) {
	switch zoneOverride = strings.TrimSpace(zoneOverride); zoneOverride {
	case "":
		zone = hostTimeZone()
	case "keep":
	default:
		zone = zoneOverride
	}
	switch keyboardOverride = strings.TrimSpace(keyboardOverride); keyboardOverride {
	case "":
		layout, variant = linuxKeyboardEnvironment(os.Getenv("XKB_DEFAULT_LAYOUT"), os.Getenv("XKB_DEFAULT_VARIANT"))
	case "keep":
	default:
		layout, variant = splitKeyboardSpec(keyboardOverride)
	}
	switch localeOverride = strings.TrimSpace(localeOverride); localeOverride {
	case "":
		locale = posixLocaleName(os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG"))
	case "keep":
	default:
		locale = localeOverride
	}
	return zone, layout, variant, locale
}

// hostTimeZone prefers TZ, then the zone /etc/localtime points into.
func hostTimeZone() string {
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" && !filepath.IsAbs(tz) {
		return tz
	}
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return ""
	}
	return zoneFromLocaltimeLink(target)
}

func zoneFromLocaltimeLink(target string) string {
	_, zone, ok := strings.Cut(filepath.ToSlash(target), "zoneinfo/")
	if !ok {
		return ""
	}
	return strings.TrimPrefix(zone, "posix/")
}

// posixLocaleName turns the first set of en_US.UTF-8 style values into
// en_US. C and POSIX mean no preference.
func posixLocaleName(values ...string) string {
	for _, value := range values {
		name, _, _ := strings.Cut(strings.TrimSpace(value), ".")
		name, _, _ = strings.Cut(name, "@")
		if name == "" || name == "C" || name == "POSIX" {
			continue
		}
		return name
	}
	return ""
}

// kvmError is a KVM problem in words: Short is the one-line summary, and the
// message says what to change.
type kvmError struct{ Short, msg string }

func (e *kvmError) Error() string { return e.msg }

// checkKVM says which of the usual KVM problems this is and what to change.
// It is a setting on this computer, not a fault in the app.
func checkKVM() error {
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err == nil {
		return f.Close()
	}
	if errors.Is(err, fs.ErrNotExist) {
		return &kvmError{Short: "This computer does not offer KVM.", msg: "this computer does not offer KVM, which Omarchy needs to run. Turn on virtualization (Intel VT-x or AMD-V, sometimes called SVM) in the firmware settings. Inside a virtual machine, turn on nested virtualization for it"}
	}
	if errors.Is(err, fs.ErrPermission) {
		return &kvmError{Short: "This account cannot use KVM.", msg: "this account is not allowed to use KVM. Add it to the kvm group with: sudo usermod -aG kvm $USER. Then sign out and back in"}
	}
	return &kvmError{Short: "KVM could not be opened.", msg: fmt.Sprintf("cannot open /dev/kvm: %v", err)}
}

// renderNodeVendors lists the PCI vendor of each DRM render node.
func renderNodeVendors(sysClassDRM string) []string {
	matches, _ := filepath.Glob(filepath.Join(sysClassDRM, "renderD*", "device", "vendor"))
	vendors := make([]string, 0, len(matches))
	for _, path := range matches {
		if data, err := os.ReadFile(path); err == nil {
			vendors = append(vendors, strings.TrimSpace(string(data)))
		}
	}
	return vendors
}

// onlyNVIDIARenderNodes reports a host whose every GPU is NVIDIA, where
// QEMU's GL, and so the import of guest Vulkan frames, must run on NVIDIA's
// driver. That driver reads LINEAR dma-bufs bound with
// glEGLImageTargetTexStorageEXT at align(width * 4, 32) whatever pitch they
// were imported with, which shears Venus frames (see the Linux spike
// findings). Hybrid laptops usually give QEMU's window the integrated GPU.
func onlyNVIDIARenderNodes(vendors []string) bool {
	if len(vendors) == 0 {
		return false
	}
	for _, vendor := range vendors {
		if vendor != "0x10de" {
			return false
		}
	}
	return true
}

// Do not guess a keyboard layout from the language or the system console.
// Multiple active layouts need an explicit override until a desktop exposes
// the current group to sandboxed applications.
func linuxKeyboardEnvironment(layout, variant string) (string, string) {
	valid := func(s string) bool {
		for _, c := range s {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
				return false
			}
		}
		return true
	}
	if layout == "" || !valid(layout) || !valid(variant) {
		return "", ""
	}
	return layout, variant
}
