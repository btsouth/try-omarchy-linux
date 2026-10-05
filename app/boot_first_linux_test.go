//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLinuxBootFirstStagesAfterReadinessAndAppliesOffline(t *testing.T) {
	cfg := newUpdateConfig(t)
	old := installFirst(t, cfg, []byte("old rootfs"))
	next := newReleaseFixture(t, "v0.3.0", []byte("new rootfs"))
	defer next.srv.Close()
	sentinel := filepath.Join(cfg.dir, "personal-disk")
	os.WriteFile(sentinel, []byte("personal disk stays"), 0600)
	stop, err := configureLinuxGuestBootFirst(cfg, next.url(), next.sumsSHA)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if cfg.startGuestUpdate == nil || !strings.Contains(installedTag(t, cfg), old.tag) {
		t.Fatal("did not boot installed guest first")
	}
	next.mu.Lock()
	requests := len(next.rootfsGT)
	next.mu.Unlock()
	if requests != 0 {
		t.Fatal("downloaded before readiness")
	}
	cfg.startGuestUpdate()
	cfg.startGuestUpdate()
	payload := filepath.Join(updatePayloadRoot(cfg.dir, "", false), next.sumsSHA)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(payload); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background payload not staged")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	if !strings.Contains(installedTag(t, cfg), old.tag) {
		t.Fatal("background job published the guest")
	}
	if state, err := readPayloadUpdateState(cfg.dir); err != nil || state != nil {
		t.Fatalf("premature rollback state: %+v %v", state, err)
	}
	next.srv.Close()
	cfg.startGuestUpdate = nil
	stopNext, err := configureLinuxGuestBootFirst(cfg, next.url(), next.sumsSHA)
	if err != nil {
		t.Fatal(err)
	}
	defer stopNext()
	if !strings.Contains(installedTag(t, cfg), next.tag) {
		t.Fatal("staged update not applied offline")
	}
	if data, _ := os.ReadFile(sentinel); string(data) != "personal disk stays" {
		t.Fatal("personal disk changed")
	}
	if state, err := readPayloadUpdateState(cfg.dir); err != nil || state == nil || !state.GuestPending {
		t.Fatalf("no rollback protection: %+v %v", state, err)
	}
}

func TestLinuxBootFirstRejectsDamagedStagedPayload(t *testing.T) {
	cfg := newUpdateConfig(t)
	old := installFirst(t, cfg, []byte("old rootfs"))
	next := newReleaseFixture(t, "v0.3.0", []byte("new rootfs"))
	defer next.srv.Close()
	payload := filepath.Join(updatePayloadRoot(cfg.dir, "", false), next.sumsSHA)
	if err := os.MkdirAll(payload, 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(payload, "SHA256SUMS"), []byte("damaged"), 0600)
	stop, err := configureLinuxGuestBootFirst(cfg, next.url(), next.sumsSHA)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if !strings.Contains(installedTag(t, cfg), old.tag) || cfg.startGuestUpdate == nil {
		t.Fatal("damaged cache replaced the installed guest")
	}
}
