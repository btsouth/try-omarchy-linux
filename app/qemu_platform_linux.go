//go:build linux

package main

const qemuAccelerator = "kvm"

// KVM runs the guest on the host CPU model, so llvmpipe gets AVX2 when the
// machine has it.
const cpuRenderingCPUModel = "host"

// KVM resets in place, so a guest reboot keeps the same QEMU and window.
const qemuExitsOnReboot = false

func platformQemuArgs(*config) []string {
	// QEMU's default monitor and parallel text consoles draw through the
	// shared GL context; on NVIDIA their cursor timer aborted QEMU.
	return []string{"-monitor", "none", "-parallel", "none"}
}

// Stock QEMU on KVM returns pages the guest frees to the host.
func freePageReportingAvailable(string) bool { return true }
