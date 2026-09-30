//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func linuxRecoveryProgress(w *linuxSetupWindow, label string) backupProgress {
	return linuxRecoveryProgressUpdates(w.update, label, time.Now)
}

// Recovery can write hundreds of thousands of chunks. Sending each one makes
// the frontend queue GTK callbacks faster than it can render them. Keep file
// transitions and completion immediate, and limit ordinary updates to 10 Hz.
func linuxRecoveryProgressUpdates(update func(linuxSetupState), label string, now func() time.Time) backupProgress {
	var last time.Time
	var lastName string
	return func(current, total int64, name string) {
		at := now()
		complete := total > 0 && current >= total
		if name == lastName && !last.IsZero() && !complete && at.Sub(last) < 100*time.Millisecond {
			return
		}
		last, lastName = at, name
		update(linuxSetupState{Status: label + ": " + filepath.Base(name), Current: current, Total: total})
	}
}

func linuxRecoveryResult(err error, success string) string {
	if errors.Is(err, errSetupCancelled) {
		return "Operation cancelled. The original VM was kept."
	}
	var space *insufficientSpaceError
	if errors.As(err, &space) {
		return fmt.Sprintf("The chosen folder needs %s free; %s is available. Choose a folder with more space and try again. Your original VM was kept.", formatGiB(space.need), formatGiB(space.have))
	}
	if err != nil {
		return "Recovery operation failed: " + err.Error()
	}
	return success
}

func showLinuxRecoveryInWindow(w *linuxSetupWindow, defaultDir, dir string) string {
	if dir == "" {
		return "Reconnect the saved data location before using recovery."
	}
	// Backups and restores stage files in folders the person picked. Record
	// each before it exists, so an interrupted run can be cleaned up later.
	stopJournal := journalLinuxStaging(defaultDir)
	defer stopJournal()
	complete := completeInstallExists(dir, "disk.raw")
	retained, booted := linuxRetainedMove(defaultDir, dir)
	found := linuxFindLeftovers(defaultDir)
	sections := []linuxSection{{Heading: "Storage", Rows: linuxStorageRows(dir, defaultDir, true)}}
	if rows := linuxLeftoverRows(found); len(rows) > 0 {
		sections = append(sections, linuxSection{Heading: "Left behind by an interrupted backup or restore", Rows: rows})
	}
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "recovery", Sections: sections,
		Status:  "Backups are .zip files saved in a folder you choose, and Try Omarchy never deletes them. Restoring makes a separate copy and keeps your current VM.",
		CanMove: complete && retained == nil, CanReset: complete, CanCleanMove: retained != nil && booted, CanCleanReset: len(linuxRetainedResetDisks(dir)) > 0,
		CanCleanLeftovers: len(linuxRemovableLeftovers(found)) > 0})
	if err != nil || answer == "back" || answer == "cancel" {
		return ""
	}
	switch answer {
	case "diagnostics":
		facts := hostFacts()
		facts["launcher.version"] = linuxAppVersion
		facts["time"] = time.Now().Format(time.RFC3339)
		path, err := writeDiagnostics(dir, facts)
		if err != nil {
			return "Could not create diagnostics: " + err.Error()
		}
		return "Diagnostics saved to " + path + ". Review the bundle before sharing it; logs can still contain local details."
	case "backup":
		result, _ := backupLinuxVM(w, dir)
		return result
	case "move":
		return moveLinuxInstallation(w, defaultDir, dir)
	case "reset":
		return resetLinuxVM(w, dir)
	case "clean-move":
		return cleanupLinuxMove(w, defaultDir, dir)
	case "clean-reset":
		return cleanupLinuxResetDisks(w, dir)
	case "clean-leftovers":
		return cleanupLinuxLeftovers(w, defaultDir)
	case "restore":
		archive, err := w.ask(context.Background(), linuxSetupState{Prompt: "restore-archive", Status: "Choose a Try Omarchy backup. Restoring will make a new copy and leave the current VM in place."})
		if err != nil || archive == "cancel" {
			return ""
		}
		if !filepath.IsAbs(archive) {
			return "Choose an absolute backup file."
		}
		parent, err := w.ask(context.Background(), linuxSetupState{Prompt: "restore-parent", Status: "Choose the folder that will hold a new restored Try Omarchy copy. Your current VM remains untouched."})
		if err != nil || parent == "cancel" {
			return ""
		}
		if !filepath.IsAbs(parent) {
			return "Choose an absolute destination folder."
		}
		destination := filepath.Join(parent, fmt.Sprintf("try-omarchy-restored-%s", time.Now().Format("20060102-150405.000000000")))
		if pathsOverlap(dir, destination) {
			return "Choose a restore destination outside the current Try Omarchy data folder."
		}
		configureSetupCancellation(false)
		linuxRecoveryActive.Store(true)
		w.update(linuxSetupState{Status: "Restoring a copy..."})
		err = restoreVMBackupProgress(archive, destination, linuxRecoveryProgress(w, "Restoring"))
		linuxRecoveryActive.Store(false)
		configureSetupCancellation(false)
		return linuxRecoveryResult(err, "Restored copy saved in the selected folder as "+filepath.Base(destination)+". On the home screen, choose Use existing data folder to open this copy. Your original VM remains available.")
	default:
		return ""
	}
}

// backupLinuxVM asks for a folder and writes a backup there. It reports
// whether a backup was saved; an empty message means the user cancelled.
func backupLinuxVM(w *linuxSetupWindow, dir string) (string, bool) {
	if _, err := os.Stat(filepath.Join(dir, "vm", "disk.raw")); err != nil {
		return "No installed VM is available to back up: " + err.Error(), false
	}
	folder, err := w.ask(context.Background(), linuxSetupState{Prompt: "backup-folder", Status: "Choose a folder outside the Try Omarchy data folder. The backup contains your guest files and settings; keep it private."})
	if err != nil || folder == "cancel" {
		return "", false
	}
	if !filepath.IsAbs(folder) {
		return "Choose an absolute backup folder.", false
	}
	destination := filepath.Join(folder, fmt.Sprintf("try-omarchy-backup-%s.zip", time.Now().Format("20060102-150405.000000000")))
	configureSetupCancellation(false)
	linuxRecoveryActive.Store(true)
	w.update(linuxSetupState{Status: "Creating backup..."})
	err = writeVMBackupProgress(dir, destination, linuxRecoveryProgress(w, "Backing up"))
	linuxRecoveryActive.Store(false)
	configureSetupCancellation(false)
	return linuxRecoveryResult(err, "Backup saved in the selected folder as "+filepath.Base(destination)+". It contains guest files and settings; keep it private."), err == nil
}

// cleanupLinuxLeftovers removes what an interrupted backup or restore left in
// folders the person chose, after showing exactly which files those are.
func cleanupLinuxLeftovers(w *linuxSetupWindow, defaultDir string) string {
	items := linuxRemovableLeftovers(linuxFindLeftovers(defaultDir))
	if len(items) == 0 {
		return "There is nothing left to remove."
	}
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: "Remove unfinished files?", Primary: "Keep them", Secondary: "Remove", Destructive: true,
		Status: linuxLeftoverPrompt(items)})
	if err != nil || answer != "secondary" {
		return "The unfinished files were kept."
	}
	var freed int64
	var failures []string
	removed := 0
	for _, item := range items {
		if err := removeLinuxLeftover(defaultDir, item.linuxLeftover); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		removed++
		freed += item.Bytes
	}
	result := fmt.Sprintf("Removed %d unfinished item(s), freeing about %s.", removed, linuxGB(freed))
	if len(failures) > 0 {
		result += " Could not remove everything: " + strings.Join(failures, " ")
	}
	return result
}
