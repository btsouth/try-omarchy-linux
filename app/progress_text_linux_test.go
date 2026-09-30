//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestLinuxStatusesSayWhatAPersonIsWaitingFor(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		updating bool
		text     string
		stage    linuxStage
	}{
		{"Preparing Omarchy...", false, "Getting ready", stagePrepare},
		{"Downloading Omarchy (1 of 5)...", false, "Getting ready", stagePrepare},
		{"Downloading Omarchy (4 of 5)...", false, "Getting ready", stagePrepare},
		{"Downloading Omarchy (5 of 5)...", false, "Downloading Omarchy", stageDownload},
		{"Downloading Omarchy (5 of 5)...", true, "Downloading the Omarchy update", stageDownload},
		{"Resuming rootfs.ext4.zst...", false, "Downloading Omarchy", stageDownload},
		{"Checking cached guest-manifest.json...", false, "Getting ready", stagePrepare},
		{"Checking cached rootfs.ext4.zst...", false, "Checking the download", stageCheck},
		{"Checking downloaded rootfs.ext4.zst...", false, "Checking the download", stageCheck},
		{"Checking the cached Omarchy system...", false, "Checking Omarchy's system files", stageCheck},
		{"Unpacking the Omarchy system...", false, "Unpacking Omarchy", stageUnpack},
		{"Checking the unpacked Omarchy system...", false, "Checking the unpacked files", stageCheck},
		{"Ready - starting Omarchy...", false, "Almost ready", stagePrepare},
		{"Preparing your Omarchy disk...", false, "Creating your Omarchy disk", stageDisk},
		{"Starting Omarchy - GPU accelerated (virgl + Venus Vulkan)", false, "Starting Omarchy", stageStart},
		{"Starting Omarchy - CPU rendering (llvmpipe)", false, "Starting Omarchy", stageStart},
		{"Booting Omarchy...", false, "Starting Omarchy", stageStart},
		{"Omarchy is starting its desktop...", false, "Loading the Omarchy desktop", stageDesktop},
		{"Preparing an Omarchy image update...", true, "Getting the Omarchy update ready", stagePrepare},
		{"Reclaiming disk space...", false, "Reclaiming disk space...", stageOther},
	} {
		if text, stage := linuxFriendlyStatus(tc.raw, tc.updating); text != tc.text || stage != tc.stage {
			t.Errorf("linuxFriendlyStatus(%q, %t) = %q %d, want %q %d", tc.raw, tc.updating, text, stage, tc.text, tc.stage)
		}
	}
}

func TestNoEngineeringTermsReachTheWindow(t *testing.T) {
	for _, raw := range []string{
		"Starting Omarchy - GPU accelerated (virgl + Venus Vulkan)",
		"Starting Omarchy - GPU accelerated OpenGL (software Vulkan)",
		"Starting Omarchy - CPU rendering (llvmpipe)",
	} {
		text, _ := linuxFriendlyStatus(raw, false)
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
	ui.setStatus("Downloading Omarchy (2 of 5)...")
	ui.setProgress(50, 100)
	if ui.state.Total != 0 || ui.state.Status != "Getting ready" {
		t.Fatalf("a small preparation step showed progress: %+v", ui.state)
	}
	ui.setStatus("Downloading Omarchy (5 of 5)...")
	ui.setProgress(1288490188, 1799401120)
	if ui.state.Total != 1799401120 || !strings.HasPrefix(ui.state.Detail, "1.2 GB of 1.7 GB") {
		t.Fatalf("the system image download must be measured: %+v", ui.state)
	}
	ui.setUpdating(true)
	ui.setStatus("Downloading Omarchy (5 of 5)...")
	if ui.state.Status != "Downloading the Omarchy update" {
		t.Fatalf("update wording: %+v", ui.state)
	}
}

func TestLinuxDesktopWaitExplainsAccountSetup(t *testing.T) {
	ui := &progressUI{lastPercent: -1}
	ui.setStatus("Omarchy is starting its desktop...")
	if !strings.Contains(ui.state.Detail, "account setup or sign in") || ui.state.Total != 0 || ui.state.Error {
		t.Fatalf("desktop wait must explain the guest input without claiming failure or measurable progress: %+v", ui.state)
	}
	ui.showDesktopTimeout("Finish account setup in the Omarchy window.")
	if ui.state.Error || !ui.state.Booting || !strings.Contains(ui.state.Detail, "account setup") {
		t.Fatalf("an unconfirmed desktop must remain a stoppable waiting state: %+v", ui.state)
	}
}
