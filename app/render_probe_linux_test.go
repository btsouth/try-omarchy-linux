//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLinuxRenderFallbackRemembered(t *testing.T) {
	dir := t.TempDir()
	qemu := filepath.Join(dir, "qemu")
	if err := os.WriteFile(qemu, []byte("runtime one"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config{dir: dir, qemu: qemu, renderMode: renderAuto, audio: "none"}
	configureLinuxRender(cfg, "driver one")
	if !cfg.useGpu {
		t.Fatal("first launch skipped GPU")
	}
	if !linuxStartupFallback(cfg) || cfg.useGpu {
		t.Fatal("failed GPU did not fall back")
	}
	recordRenderResult(cfg)
	configureLinuxRender(cfg, "driver one")
	if cfg.useGpu {
		t.Fatal("next launch retried failed GPU")
	}
	cfg.renderMode = renderGPU
	configureLinuxRender(cfg, "driver one")
	if !cfg.useGpu {
		t.Fatal("explicit GPU did not retry")
	}
	cfg.renderMode = renderAuto
	configureLinuxRender(cfg, "driver two")
	if !cfg.useGpu {
		t.Fatal("driver change did not retry")
	}
	if err := os.WriteFile(qemu, []byte("runtime two, new version"), 0600); err != nil {
		t.Fatal(err)
	}
	configureLinuxRender(cfg, "driver one")
	if !cfg.useGpu {
		t.Fatal("runtime change did not retry")
	}
	// Explicit CPU mode must not create a failure record.
	if err := os.Remove(filepath.Join(dir, renderProbeFilename)); err != nil {
		t.Fatal(err)
	}
	cfg.renderMode = renderCPU
	configureLinuxRender(cfg, "driver one")
	recordRenderResult(cfg)
	if probe, err := loadRenderProbe(dir); err != nil || probe != nil {
		t.Fatalf("explicit CPU recorded a failure: %v %v", probe, err)
	}
	cfg.renderMode = renderAuto
	configureLinuxRender(cfg, "driver one")
	cfg.useGpu = false
	recordRenderResult(cfg)
	probe, _ := loadRenderProbe(dir)
	probe.RecordedAt = time.Now().Add(-25 * time.Hour)
	if err := saveRenderProbe(dir, *probe); err != nil {
		t.Fatal(err)
	}
	configureLinuxRender(cfg, "driver one")
	if !cfg.useGpu {
		t.Fatal("expired fallback did not retry")
	}
	recordRenderResult(cfg)
	configureLinuxRender(cfg, "driver one")
	if !cfg.useGpu {
		t.Fatal("successful GPU result was not remembered")
	}
}

func TestLinuxGraphicsIdentityChanges(t *testing.T) {
	dir := t.TempDir()
	kernel, driver := filepath.Join(dir, "kernel"), filepath.Join(dir, "driver.so")
	for _, path := range []string{kernel, driver} {
		if err := os.WriteFile(path, []byte("v1"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	identity := func(settings string) string { return linuxGraphicsIdentity(kernel, []string{driver}, settings) }
	first := identity("venus=true")
	if first != identity("venus=true") {
		t.Fatal("identity changed without a driver change")
	}
	if first == identity("venus=false") {
		t.Fatal("graphics setting change ignored")
	}
	if err := os.WriteFile(driver, []byte("new driver version"), 0600); err != nil {
		t.Fatal(err)
	}
	second := identity("venus=true")
	if first == second {
		t.Fatal("userspace driver update ignored")
	}
	if err := os.WriteFile(kernel, []byte("v2"), 0600); err != nil {
		t.Fatal(err)
	}
	if second == identity("venus=true") {
		t.Fatal("kernel update ignored")
	}
}
