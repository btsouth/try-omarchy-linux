//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var linuxPowerSupplyRoot = "/sys/class/power_supply"

func linuxPowerValue(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func linuxPowerNumber(dir, name string) (int64, bool) {
	value, err := strconv.ParseInt(linuxPowerValue(dir, name), 10, 64)
	return value, err == nil && value >= 0
}

func linuxBatterySnapshot(root string) batterySnapshot {
	snapshot, _ := linuxBatterySnapshotAndDetails(root)
	return snapshot
}

func linuxBatterySnapshotAndDetails(root string) (batterySnapshot, batteryDetails) {
	snapshot := batterySnapshot{Type: "state", State: "unknown"}
	entries, err := os.ReadDir(root)
	if err != nil {
		return snapshot, batteryDetails{}
	}
	for _, entry := range entries {
		dir := filepath.Join(root, entry.Name())
		switch linuxPowerValue(dir, "type") {
		case "Mains", "USB", "USB_C", "USB_PD":
			if linuxPowerValue(dir, "online") == "1" {
				snapshot.ACConnected = true
			}
		}
	}
	for _, entry := range entries {
		dir := filepath.Join(root, entry.Name())
		if linuxPowerValue(dir, "type") != "Battery" || linuxPowerValue(dir, "present") == "0" {
			continue
		}
		snapshot.Present = true
		if capacity, ok := linuxPowerNumber(dir, "capacity"); ok && capacity <= 100 {
			percent := int(capacity)
			snapshot.Percentage = &percent
		}
		switch strings.ToLower(linuxPowerValue(dir, "status")) {
		case "charging":
			snapshot.State = "charging"
		case "discharging":
			snapshot.State = "discharging"
		case "full":
			snapshot.State = "full"
		case "not charging":
			snapshot.State = "not-charging"
		}
		if snapshot.State == "discharging" {
			if seconds, ok := linuxPowerNumber(dir, "time_to_empty_now"); ok && seconds <= 30*24*60*60 {
				value := int(seconds)
				snapshot.TimeToEmptySeconds = &value
			}
		} else if snapshot.State == "charging" {
			if seconds, ok := linuxPowerNumber(dir, "time_to_full_now"); ok && seconds <= 30*24*60*60 {
				value := int(seconds)
				snapshot.TimeToFullSeconds = &value
			}
		}
		// Windows queries the first battery device rather than aggregating
		// physical details. Use the same battery for Linux state and details.
		return snapshot, linuxBatteryDetails(dir)
	}
	return snapshot, batteryDetails{}
}

func hostBatteryLine() (string, error) {
	snapshot, details := linuxBatterySnapshotAndDetails(linuxPowerSupplyRoot)
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("encode battery: %w", err)
	}
	return "battery " + string(data) + "\n" + encodeBatteryDetailsLine(details), nil
}
