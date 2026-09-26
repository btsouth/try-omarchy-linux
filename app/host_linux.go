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
// guest alone. The keyboard layout is not read from the host yet.
func hostLocale(zoneOverride, keyboardOverride, localeOverride string) (zone, layout, variant, locale string) {
	switch zoneOverride = strings.TrimSpace(zoneOverride); zoneOverride {
	case "":
		zone = hostTimeZone()
	case "keep":
	default:
		zone = zoneOverride
	}
	switch keyboardOverride = strings.TrimSpace(keyboardOverride); keyboardOverride {
	case "", "keep":
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

// checkKVM explains the two ways /dev/kvm is usually unavailable.
func checkKVM() error {
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err == nil {
		return f.Close()
	}
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("this computer has no /dev/kvm. Turn on virtualization (Intel VT-x or AMD-V) in the firmware settings")
	}
	if errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("this account cannot use /dev/kvm. Add it to the kvm group (sudo usermod -aG kvm $USER), then sign out and back in")
	}
	return fmt.Errorf("cannot open /dev/kvm: %w", err)
}
