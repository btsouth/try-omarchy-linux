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
		return uiText("recovery.linux.operation_cancelled_the_original_vm_was_kept")
	}
	var space *insufficientSpaceError
	if errors.As(err, &space) {
		return uiTextWith("recovery.linux.the_chosen_folder_needs_free_is_available_choose", map[string]string{"space_need": formatGiB(space.need), "space_have": formatGiB(space.have)})
	}
	if err != nil {
		return uiTextWith("recovery.linux.recovery_operation_failed", map[string]string{"error": err.Error()})
	}
	return success
}

func showLinuxRecoveryInWindow(w *linuxSetupWindow, defaultDir, dir string) string {
	if dir == "" {
		return uiText("recovery.linux.reconnect_the_saved_data_location_before_using_recovery")
	}
	// Backups and restores stage files in folders the person picked. Record
	// each before it exists, so an interrupted run can be cleaned up later.
	stopJournal := journalLinuxStaging(defaultDir)
	defer stopJournal()
	complete := completeInstallExists(dir, "disk.raw")
	retained, booted := linuxRetainedMove(defaultDir, dir)
	found := linuxFindLeftovers(defaultDir)
	sections := []linuxSection{{Heading: uiText("settings.linux.storage"), Rows: linuxStorageRows(dir, defaultDir, true)}}
	if rows := linuxLeftoverRows(found); len(rows) > 0 {
		sections = append(sections, linuxSection{Heading: uiText("recovery.linux.left_behind_by_an_interrupted_backup_or_restore"), Rows: rows})
	}
	page := linuxSetupState{Prompt: "recovery", Sections: sections,
		Status:  uiText("recovery.linux.snapshots_save_this_vm_inside_its_folder_so"),
		CanMove: complete && retained == nil, CanReset: complete, CanCleanMove: retained != nil && booted, CanCleanReset: len(linuxRetainedResetDisks(dir)) > 0,
		CanCleanLeftovers: len(linuxRemovableLeftovers(found)) > 0, CanSnapshot: complete, CanMigrate: complete, CanCleanRollback: len(linuxRollbackKept(dir)) > 0}
	var answer string
	for {
		var err error
		answer, err = w.ask(context.Background(), page)
		if err != nil || answer == "back" || answer == "cancel" {
			return ""
		}
		if answer != "migration" {
			break
		}
		if !complete {
			return uiText("recovery.linux.there_is_no_complete_vm_here_to_export")
		}
		if _, err := w.ask(context.Background(), linuxMigrationState()); err != nil {
			return ""
		}
	}
	switch answer {
	case "diagnostics":
		facts := hostFacts()
		facts["launcher.version"] = linuxAppVersion
		facts["time"] = time.Now().Format(time.RFC3339)
		path, err := writeDiagnostics(dir, facts)
		if err != nil {
			return uiTextWith("tray.linux.could_not_create_diagnostics", map[string]string{"error": err.Error()})
		}
		return uiTextWith("recovery.linux.diagnostics_saved_to_review_the_bundle_before_sharing", map[string]string{"path": path})
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
	case "snapshots":
		return showLinuxSnapshots(w, dir)
	case "clean-rollback":
		return cleanupLinuxRollbackKept(w, dir)
	case "restore":
		archive, err := w.ask(context.Background(), linuxSetupState{Prompt: "restore-archive", Status: uiText("recovery.linux.choose_a_try_omarchy_backup_restoring_will_make")})
		if err != nil || archive == "cancel" {
			return ""
		}
		if !filepath.IsAbs(archive) {
			return uiText("recovery.linux.choose_an_absolute_backup_file")
		}
		parent, err := w.ask(context.Background(), linuxSetupState{Prompt: "restore-parent", Status: uiText("recovery.linux.choose_the_folder_that_will_hold_a_new")})
		if err != nil || parent == "cancel" {
			return ""
		}
		if !filepath.IsAbs(parent) {
			return uiText("recovery.linux.choose_an_absolute_destination_folder")
		}
		destination := filepath.Join(parent, fmt.Sprintf("try-omarchy-restored-%s", time.Now().Format("20060102-150405.000000000")))
		if pathsOverlap(dir, destination) {
			return uiText("recovery.linux.choose_a_restore_destination_outside_the_current_try")
		}
		configureSetupCancellation(false)
		linuxRecoveryActive.Store(true)
		w.update(linuxSetupState{Status: uiText("recovery.linux.restoring_a_copy")})
		err = restoreVMBackupProgress(archive, destination, linuxRecoveryProgress(w, uiText("recovery.linux.restoring")))
		linuxRecoveryActive.Store(false)
		configureSetupCancellation(false)
		return linuxRecoveryResult(err, uiTextWith("recovery.linux.restored_copy_saved_in_the_selected_folder_as", map[string]string{"path": filepath.Base(destination)}))
	default:
		return ""
	}
}

func linuxMigrationState() linuxSetupState {
	return linuxSetupState{Prompt: "migration", Title: uiText("recovery.linux.move_to_installed_omarchy"),
		Status: uiText("recovery.linux.bring_the_changes_you_made_in_this_trial"),
		Sections: []linuxSection{{Rows: []linuxRow{
			{Title: uiText("recovery.linux.1_export_inside_the_trial"), Detail: uiText("recovery.linux.launch_omarchy_open_a_terminal_and_run_try")},
			{Title: uiText("recovery.linux.2_keep_the_archive_outside_the_trial"), Detail: uiText("recovery.linux.the_command_saves_omarchy_export_date_tar_gz")},
			{Title: uiText("recovery.linux.3_install_omarchy"), Detail: uiText("recovery.linux.download_the_iso_from_omarchy_org_and_follow")},
			{Title: uiText("recovery.linux.4_import_on_installed_omarchy"), Detail: uiText("recovery.linux.run_update_omarchy_on_the_new_desktop_before")},
		}}},
		HelpURL: "https://github.com/btsouth/try-omarchy-linux/blob/master/docs/MIGRATION.md",
		Actions: []linuxAction{{Label: uiText("launcher.linux.back"), Reply: "close"}}}
}

// backupLinuxVM asks for a folder and writes a backup there. It reports
// whether a backup was saved; an empty message means the user cancelled.
func backupLinuxVM(w *linuxSetupWindow, dir string) (string, bool) {
	if _, err := os.Stat(filepath.Join(dir, "vm", "disk.raw")); err != nil {
		return uiTextWith("recovery.linux.no_installed_vm_is_available_to_back_up", map[string]string{"error": err.Error()}), false
	}
	folder, err := w.ask(context.Background(), linuxSetupState{Prompt: "backup-folder", Status: uiText("recovery.linux.choose_a_folder_outside_the_try_omarchy_data")})
	if err != nil || folder == "cancel" {
		return "", false
	}
	if !filepath.IsAbs(folder) {
		return uiText("recovery.linux.choose_an_absolute_backup_folder"), false
	}
	destination := filepath.Join(folder, fmt.Sprintf("try-omarchy-backup-%s.zip", time.Now().Format("20060102-150405.000000000")))
	configureSetupCancellation(false)
	linuxRecoveryActive.Store(true)
	w.update(linuxSetupState{Status: uiText("recovery.linux.creating_backup")})
	err = writeVMBackupProgress(dir, destination, linuxRecoveryProgress(w, uiText("recovery.linux.backing_up")))
	linuxRecoveryActive.Store(false)
	configureSetupCancellation(false)
	return linuxRecoveryResult(err, uiTextWith("recovery.linux.backup_saved_in_the_selected_folder_as_it", map[string]string{"path": filepath.Base(destination)})), err == nil
}

// cleanupLinuxLeftovers removes what an interrupted backup or restore left in
// folders the person chose, after showing exactly which files those are.
func cleanupLinuxLeftovers(w *linuxSetupWindow, defaultDir string) string {
	items := linuxRemovableLeftovers(linuxFindLeftovers(defaultDir))
	if len(items) == 0 {
		return uiText("recovery.linux.there_is_nothing_left_to_remove")
	}
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: uiText("recovery.linux.remove_unfinished_files"), Primary: uiText("recovery.linux.keep_them"), Secondary: uiText("settings.linux.remove"), Destructive: true,
		Status: linuxLeftoverPrompt(items)})
	if err != nil || answer != "secondary" {
		return uiText("recovery.linux.the_unfinished_files_were_kept")
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
	result := uiTextWith("recovery.linux.removed_unfinished_item_s_freeing_about", map[string]string{"removed": fmt.Sprintf("%d", removed), "freed": linuxGB(freed)})
	if len(failures) > 0 {
		result += uiTextWith("recovery.linux.could_not_remove_everything", map[string]string{"names": strings.Join(failures, " ")})
	}
	return result
}
