//go:build linux

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// The launcher reports engineering steps ("Starting Omarchy - GPU accelerated
// (virgl + Venus Vulkan)"). The window says what a person is waiting for, and
// only shows a bar when there is real progress to measure. The terminal and
// the log keep the raw text.

type linuxStage int

const (
	stageOther    linuxStage = iota
	stagePrepare             // small files, checks and hand-offs: nothing to measure
	stageDownload            // the system image, measured in bytes
	stageCheck               // verifying what was downloaded or unpacked
	stageUnpack
	stageDisk
	stageStart
	stageDesktop
)

var linuxDownloadStep = regexp.MustCompile(`^Downloading Omarchy \((\d+) of (\d+)\)\.\.\.$`)

// linuxFriendlyStatus translates one launcher message. Of the five files a
// first setup downloads, four are a few hundred megabytes together at most and
// only the last, the system image, is worth a bar; calling them "1 of 5" made
// the first four steps look like a fifth of the job each.
func linuxFriendlyStatus(raw string, updating bool) (string, linuxStage) {
	download, prepare := "Downloading Omarchy", "Getting ready"
	if updating {
		download, prepare = "Downloading the Omarchy update", "Getting the Omarchy update ready"
	}
	if m := linuxDownloadStep.FindStringSubmatch(raw); m != nil {
		if m[1] == m[2] {
			return download, stageDownload
		}
		return prepare, stagePrepare
	}
	switch {
	case raw == "Preparing Omarchy...":
		return prepare, stagePrepare
	case raw == "Preparing an Omarchy image update...":
		return "Getting the Omarchy update ready", stagePrepare
	case strings.HasPrefix(raw, "Resuming "):
		return download, stageDownload
	case strings.HasPrefix(raw, "Checking cached ") || strings.HasPrefix(raw, "Checking downloaded "):
		name := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(raw, "Checking cached "), "Checking downloaded "), "...")
		if filepath.Base(name) == "rootfs.ext4.zst" {
			return "Checking the download", stageCheck
		}
		return prepare, stagePrepare
	case raw == "Checking the cached Omarchy system...":
		return "Checking Omarchy's system files", stageCheck
	case raw == "Unpacking the Omarchy system...":
		return "Unpacking Omarchy", stageUnpack
	case raw == "Checking the unpacked Omarchy system...":
		return "Checking the unpacked files", stageCheck
	case raw == "Ready - starting Omarchy...":
		return "Almost ready", stagePrepare
	case raw == "Preparing your Omarchy disk...":
		return "Creating your Omarchy disk", stageDisk
	case strings.HasPrefix(raw, "Starting Omarchy") || raw == "Booting Omarchy...":
		return "Starting Omarchy", stageStart
	case raw == "Omarchy is starting its desktop...":
		return "Loading the Omarchy desktop", stageDesktop
	}
	return raw, stageOther
}

// linuxProgressDetail is the line under the bar. A download says how much of
// it is done and that it survives an interruption; other steps let the bar speak.
func linuxProgressDetail(stage linuxStage, current, total int64) string {
	if stage == stageDesktop {
		return "Finish first-time account setup or sign in in the Omarchy window. This window closes when the desktop is ready."
	}
	if stage == stageDownload && total > 0 {
		return fmt.Sprintf("%s of %s. If this stops, it continues where it left off.", linuxGB(current), linuxGB(total))
	}
	return ""
}
