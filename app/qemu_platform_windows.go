//go:build windows

package main

// The Windows Hypervisor Platform accelerator.
const qemuAccelerator = "whpx"

// CPU rendering runs stock QEMU with the fastest flags upstream WHPX
// survives: any XSAVE/AVX feature panics the guest kernel.
const cpuRenderingCPUModel = "qemu64,+ssse3,+sse4.1,+sse4.2,+popcnt,+aes"

// In-guest reboot/poweroff wedges upstream WHPX (vCPUs never return from
// system reset). QEMU exits instead; the supervisor relaunches on reset.
const qemuExitsOnReboot = true

func platformQemuArgs(*config) []string { return nil }

// The bundled r19 runtime replaces reported free pages with demand-zero
// Windows backing before acknowledging Linux. Earlier Windows QEMU builds
// cannot discard these pages and spam errors when reporting is enabled.
func freePageReportingAvailable(qemu string) bool {
	return runtimeHasPatch(qemu, "patches/qemu/0015-reclaim-free-guest-pages-on-whpx.patch")
}
