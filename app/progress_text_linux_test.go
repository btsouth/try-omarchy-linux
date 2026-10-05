//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestLinuxStatusesSayWhatAPersonIsWaitingFor(t *testing.T) {
	for _, tc := range []struct {
		key      string
		values   map[string]string
		updating bool
		text     string
		stage    linuxStage
	}{
		{"launcher.linux.preparing_omarchy", nil, false, "Getting ready", stagePrepare},
		{"status.downloading_omarchy", map[string]string{"part": "1", "total": "5"}, false, "Getting ready", stagePrepare},
		{"status.downloading_omarchy", map[string]string{"part": "4", "total": "5"}, false, "Getting ready", stagePrepare},
		{"status.downloading_omarchy", map[string]string{"part": "5", "total": "5"}, false, "Downloading Omarchy", stageDownload},
		{"status.downloading_omarchy", map[string]string{"part": "5", "total": "5"}, true, "Downloading the Omarchy update", stageDownload},
		{"status.resuming_file", map[string]string{"file": "rootfs.ext4.zst"}, false, "Downloading Omarchy", stageDownload},
		{"status.checking_cached_file", map[string]string{"file": "guest-manifest.json"}, false, "Getting ready", stagePrepare},
		{"status.checking_cached_file", map[string]string{"file": "rootfs.ext4.zst"}, false, "Checking the download", stageCheck},
		{"status.checking_downloaded_file", map[string]string{"file": "rootfs.ext4.zst"}, false, "Checking the download", stageCheck},
		{"status.checking_cached_system", nil, false, "Checking Omarchy's system files", stageCheck},
		{"status.unpacking_system", nil, false, "Unpacking Omarchy", stageUnpack},
		{"status.checking_unpacked_system", nil, false, "Checking the unpacked files", stageCheck},
		{"status.ready_starting", nil, false, "Almost ready", stagePrepare},
		{"status.preparing_disk", nil, false, "Creating your Omarchy disk", stageDisk},
		{"status.linux.starting", map[string]string{"mode": "GPU accelerated (virgl + Venus Vulkan)"}, false, "Starting Omarchy", stageStart},
		{"status.linux.starting", map[string]string{"mode": "CPU rendering (llvmpipe)"}, false, "Starting Omarchy", stageStart},
		{"status.linux.booting", nil, false, "Starting Omarchy", stageStart},
		{"status.linux.desktop_starting", nil, false, "Loading the Omarchy desktop", stageDesktop},
		{"status.preparing_image_update", nil, true, "Getting the Omarchy update ready", stagePrepare},
		{"status.reclaiming", nil, false, "Reclaiming disk space...", stageOther},
	} {
		if text, stage := linuxFriendlyStatus(tc.key, tc.values, tc.updating); text != tc.text || stage != tc.stage {
			t.Errorf("linuxFriendlyStatus(%q, %t) = %q %d, want %q %d", tc.key, tc.updating, text, stage, tc.text, tc.stage)
		}
	}
}

func TestNoEngineeringTermsReachTheWindow(t *testing.T) {
	for _, raw := range []string{
		"Starting Omarchy - GPU accelerated (virgl + Venus Vulkan)",
		"Starting Omarchy - GPU accelerated OpenGL (software Vulkan)",
		"Starting Omarchy - CPU rendering (llvmpipe)",
	} {
		text, _ := linuxFriendlyStatus("status.linux.starting", map[string]string{"mode": strings.TrimPrefix(raw, "Starting Omarchy - ")}, false)
		for _, jargon := range []string{"virgl", "Venus", "Vulkan", "llvmpipe", "GPU", "OpenGL"} {
			if strings.Contains(text, jargon) {
				t.Errorf("%q leaked %q into the window: %q", raw, jargon, text)
			}
		}
	}
}

func TestLinuxProgressDetailReportsTheDownloadOnly(t *testing.T) {
	got := linuxProgressDetail(stageDownload, 1288490188, 1799401120)
	if got != "1.2 GB of 1.7 GB. If this stops, it continues where it left off." {
		t.Fatalf("download detail: %q", got)
	}
	for _, stage := range []linuxStage{stagePrepare, stageCheck, stageUnpack, stageDisk, stageStart, stageOther} {
		if linuxProgressDetail(stage, 5, 10) != "" {
			t.Fatalf("stage %d should not add detail", stage)
		}
	}
	if linuxProgressDetail(stageDownload, 0, 0) != "" {
		t.Fatal("no total, no detail")
	}
}

func TestPreparationStepsShowNoBarButRealStagesDo(t *testing.T) {
	ui := &progressUI{lastPercent: -1}
	ui.setCatalogStatus("status.downloading_omarchy", map[string]string{"part": "2", "total": "5"})
	ui.setProgress(50, 100)
	if ui.state.Total != 0 || ui.state.Status != "Getting ready" {
		t.Fatalf("a small preparation step showed progress: %+v", ui.state)
	}
	ui.setCatalogStatus("status.downloading_omarchy", map[string]string{"part": "5", "total": "5"})
	ui.setProgress(1288490188, 1799401120)
	if ui.state.Total != 1799401120 || !strings.HasPrefix(ui.state.Detail, "1.2 GB of 1.7 GB") {
		t.Fatalf("the system image download must be measured: %+v", ui.state)
	}
	ui.setUpdating(true)
	ui.setCatalogStatus("status.downloading_omarchy", map[string]string{"part": "5", "total": "5"})
	if ui.state.Status != "Downloading the Omarchy update" {
		t.Fatalf("update wording: %+v", ui.state)
	}
}

func TestLinuxDesktopWaitExplainsAccountSetup(t *testing.T) {
	ui := &progressUI{lastPercent: -1}
	ui.setCatalogStatus("status.linux.desktop_starting", nil)
	if !strings.Contains(ui.state.Detail, "account setup or sign in") || ui.state.Total != 0 || ui.state.Error {
		t.Fatalf("desktop wait must explain the guest input without claiming failure or measurable progress: %+v", ui.state)
	}
	ui.showDesktopTimeout("Finish account setup in the Omarchy window.")
	if ui.state.Error || !ui.state.Booting || !strings.Contains(ui.state.Detail, "account setup") {
		t.Fatalf("an unconfirmed desktop must remain a stoppable waiting state: %+v", ui.state)
	}
}
