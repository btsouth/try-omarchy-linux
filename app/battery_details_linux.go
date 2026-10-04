//go:build linux

package main

import "math"

// The shared guest protocol accepts positive signed 32-bit physical readings.
// Omit unavailable or out-of-range fields without losing the battery's state.
func linuxBatteryPhysicalNumber(dir, name string) *int64 {
	value, ok := linuxPowerNumber(dir, name)
	if !ok || value == 0 || value > math.MaxInt32 {
		return nil
	}
	return &value
}

func linuxBatteryEnergy(dir, energyName, chargeName string, voltage int64) *int64 {
	if energy := linuxBatteryPhysicalNumber(dir, energyName); energy != nil {
		return energy
	}
	charge, ok := linuxPowerNumber(dir, chargeName)
	if !ok || charge == 0 || voltage <= 0 || charge > math.MaxInt64/voltage {
		return nil
	}
	// Sysfs uses microamp-hours and microvolts; the bridge needs microwatt-hours.
	energy := charge * voltage / 1000000
	if energy == 0 || energy > math.MaxInt32 {
		return nil
	}
	return &energy
}

func linuxBatteryDetails(dir string) batteryDetails {
	// Prefer the battery's nominal design voltage so full/design capacities
	// stay stable as its live voltage changes. Fall back when it is unavailable.
	voltage, ok := linuxPowerNumber(dir, "voltage_min_design")
	if !ok || voltage == 0 {
		voltage, _ = linuxPowerNumber(dir, "voltage_now")
	}
	chemistry := batteryDetailText(linuxPowerValue(dir, "technology"))
	switch chemistry {
	case "Li-ion":
		chemistry = "LION"
	case "Li-poly":
		chemistry = "LIPO"
	case "Unknown":
		chemistry = ""
	}
	return batteryDetails{
		EnergyFullMicroWh:       linuxBatteryEnergy(dir, "energy_full", "charge_full", voltage),
		EnergyFullDesignMicroWh: linuxBatteryEnergy(dir, "energy_full_design", "charge_full_design", voltage),
		CycleCount:              linuxBatteryPhysicalNumber(dir, "cycle_count"),
		Manufacturer:            batteryDetailText(linuxPowerValue(dir, "manufacturer")),
		ModelName:               batteryDetailText(linuxPowerValue(dir, "model_name")),
		Chemistry:               chemistry,
	}
}
