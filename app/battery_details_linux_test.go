//go:build linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLinuxPowerSupplies(t *testing.T, root string, supplies map[string]map[string]string) {
	t.Helper()
	for device, fields := range supplies {
		dir := filepath.Join(root, device)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for name, value := range fields {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(value+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestLinuxBatteryDetails(t *testing.T) {
	for _, tt := range []struct {
		name     string
		supplies map[string]map[string]string
		present  bool
		percent  int
		details  string
	}{
		{
			name: "energy based",
			supplies: map[string]map[string]string{"BAT0": {
				"type": "Battery", "present": "1", "capacity": "57",
				"energy_full": "50000000", "energy_full_design": "60000000", "cycle_count": "213",
				"manufacturer": " ACME Corp ", "model_name": "Battery A", "technology": "Li-ion",
				// Native energy takes precedence over charge-based estimates.
				"charge_full": "1", "charge_full_design": "1", "voltage_now": "12000000",
			}},
			present: true, percent: 57,
			details: `{"energyFullDesignMicroWh":60000000,"energyFullMicroWh":50000000,"cycleCount":213,"manufacturer":"ACME Corp","modelName":"Battery A","chemistry":"LION"}`,
		},
		{
			name: "charge based with design voltage",
			supplies: map[string]map[string]string{"BAT0": {
				"type": "Battery", "capacity": "57", "charge_full": "4000000", "charge_full_design": "5000000",
				"voltage_min_design": "12000000", "voltage_now": "13000000", "technology": "Li-poly",
			}},
			present: true, percent: 57,
			details: `{"energyFullDesignMicroWh":60000000,"energyFullMicroWh":48000000,"chemistry":"LIPO"}`,
		},
		{
			name: "charge based with live voltage",
			supplies: map[string]map[string]string{"BAT0": {
				"type": "Battery", "capacity": "57", "charge_full": "4000001", "charge_full_design": "5000000",
				"voltage_min_design": "0", "voltage_now": "11000001", "technology": "NiMH",
			}},
			present: true, percent: 57,
			details: `{"energyFullDesignMicroWh":55000005,"energyFullMicroWh":44000015,"chemistry":"NiMH"}`,
		},
		{
			name: "two batteries pick first for both state and details",
			supplies: map[string]map[string]string{
				"BAT1": {"type": "Battery", "capacity": "80", "energy_full": "70000000", "cycle_count": "900", "model_name": "Second"},
				"BAT0": {"type": "Battery", "capacity": "57", "energy_full": "50000000", "cycle_count": "213", "model_name": "First"},
			},
			present: true, percent: 57,
			details: `{"energyFullMicroWh":50000000,"cycleCount":213,"modelName":"First"}`,
		},
		{
			name: "absent first battery",
			supplies: map[string]map[string]string{
				"BAT0": {"type": "Battery", "present": "0", "energy_full": "50000000", "manufacturer": "Absent"},
				"BAT1": {"type": "Battery", "capacity": "80", "energy_full": "70000000", "technology": "NiCd"},
			},
			present: true, percent: 80,
			details: `{"energyFullMicroWh":70000000,"chemistry":"NiCd"}`,
		},
		{
			name: "missing fields keep available details",
			supplies: map[string]map[string]string{"BAT0": {
				"type": "Battery", "capacity": "57", "energy_full": "50000000", "manufacturer": "ACME",
			}},
			present: true, percent: 57, details: `{"energyFullMicroWh":50000000,"manufacturer":"ACME"}`,
		},
		{
			name: "charge without voltage is not energy",
			supplies: map[string]map[string]string{"BAT0": {
				"type": "Battery", "capacity": "57", "charge_full": "4000000", "charge_full_design": "5000000",
			}},
			present: true, percent: 57, details: `{}`,
		},
		{
			name: "malformed or unsupported details are omitted",
			supplies: map[string]map[string]string{"BAT0": {
				"type": "Battery", "capacity": "57", "energy_full": "invalid", "energy_full_design": "-1", "cycle_count": "0",
				"manufacturer": "bad\nname", "model_name": strings.Repeat("x", 128), "technology": "Unknown",
			}},
			present: true, percent: 57, details: `{}`,
		},
		{
			name: "protocol integer bounds",
			supplies: map[string]map[string]string{"BAT0": {
				"type": "Battery", "capacity": "57", "energy_full": "2147483647", "energy_full_design": "2147483648", "cycle_count": "2147483648",
			}},
			present: true, percent: 57, details: `{"energyFullMicroWh":2147483647}`,
		},
		{
			name: "conversion overflow",
			supplies: map[string]map[string]string{"BAT0": {
				"type": "Battery", "capacity": "57", "charge_full": "9223372036854775807", "charge_full_design": "500000000",
				"voltage_now": "12000000",
			}},
			present: true, percent: 57, details: `{}`,
		},
		{
			name:     "no battery",
			supplies: map[string]map[string]string{"AC": {"type": "Mains", "online": "1", "manufacturer": "Not a battery"}},
			details:  `{}`,
		},
		{
			name:     "only absent battery",
			supplies: map[string]map[string]string{"BAT0": {"type": "Battery", "present": "0", "energy_full": "50000000"}},
			details:  `{}`,
		},
		{name: "empty sysfs", details: `{}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeLinuxPowerSupplies(t, root, tt.supplies)
			before := linuxPowerSupplyRoot
			linuxPowerSupplyRoot = root
			t.Cleanup(func() { linuxPowerSupplyRoot = before })
			line, err := hostBatteryLine()
			if err != nil {
				t.Fatal(err)
			}
			stateLine, detailsLine, ok := strings.Cut(line, "\n")
			if !ok || detailsLine != "battery-details "+tt.details+"\n" {
				t.Fatalf("battery details line: %q, want %s", detailsLine, tt.details)
			}
			var state batterySnapshot
			if err := json.Unmarshal([]byte(strings.TrimPrefix(stateLine, "battery ")), &state); err != nil {
				t.Fatal(err)
			}
			if state.Present != tt.present || (tt.present && (state.Percentage == nil || *state.Percentage != tt.percent)) || (!tt.present && state.Percentage != nil) {
				t.Fatalf("battery state: %+v", state)
			}
		})
	}
}

func TestLinuxBatteryDetailsDisappearAndRefresh(t *testing.T) {
	root := t.TempDir()
	before := linuxPowerSupplyRoot
	linuxPowerSupplyRoot = root
	t.Cleanup(func() { linuxPowerSupplyRoot = before })
	writeLinuxPowerSupplies(t, root, map[string]map[string]string{"BAT0": {
		"type": "Battery", "present": "1", "capacity": "57", "model_name": "Old",
	}})
	for _, model := range []string{"Old", "", "New"} {
		present := "1"
		if model == "" {
			present = "0"
		}
		writeLinuxPowerSupplies(t, root, map[string]map[string]string{"BAT0": {"present": present, "model_name": model}})
		line, err := hostBatteryLine()
		want := "battery-details {}\n"
		if model != "" {
			want = "battery-details {\"modelName\":\"" + model + "\"}\n"
		}
		if err != nil || !strings.HasSuffix(line, want) {
			t.Fatalf("model %q: %q, %v", model, line, err)
		}
	}
	linuxPowerSupplyRoot = filepath.Join(root, "missing")
	line, err := hostBatteryLine()
	if err != nil || !strings.Contains(line, `"present":false`) || !strings.HasSuffix(line, "battery-details {}\n") {
		t.Fatalf("unavailable sysfs: %q, %v", line, err)
	}
}
