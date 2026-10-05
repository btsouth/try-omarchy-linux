//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Snapshots keep the VM's complete bootable state inside its own folder: the
// disk, Omarchy's system files and the settings. The store, its integrity
// checks and the journaled roll back are shared with the Windows launcher.
// This file is the Linux surface: the pages, startup recovery, and the
// previous state a roll back keeps until the person removes it.

var linuxRollbackStage = regexp.MustCompile(`^\.snapshot-rollback-[0-9a-f]{32}$`)

// recoverLinuxSnapshots finishes or undoes an interrupted roll back, then
// clears the partial snapshot files an interrupted create or delete left.
// It runs before anything reads the VM's files, and needs Omarchy stopped.
func recoverLinuxSnapshots(dir string) error {
	if err := recoverCheckpointRollback(dir); err != nil {
		return uiError(uiTextWith("snapshots.linux.finishing_an_interrupted_roll_back", map[string]string{"error": fmt.Sprint(err)}), err)
	}
	if err := (checkpointStore{installation: dir}).Recover(); err != nil {
		return uiError(uiTextWith("snapshots.linux.clearing_an_interrupted_snapshot", map[string]string{"error": fmt.Sprint(err)}), err)
	}
	// A roll back prepares the snapshot in a private folder, publishes it as
	// its stage and only then writes its journal. With no journal, nothing
	// was moved out of the VM yet: a preparation folder, or a stage whose
	// data folder is still empty, only holds an unused copy of the snapshot.
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			abandoned := entry.IsDir() && strings.HasPrefix(entry.Name(), ".snapshot-preparing-")
			if entry.IsDir() && linuxRollbackStage.MatchString(entry.Name()) {
				kept, err := os.ReadDir(filepath.Join(path, "data"))
				abandoned = err == nil && len(kept) == 0
			}
			if abandoned && validateMovePath(path) == nil {
				if err := os.RemoveAll(path); err != nil {
					logf("snapshots: could not remove %s: %v", path, err)
				}
			}
		}
	}
	return nil
}

// linuxSnapshotRecoveryPending reports whether an interrupted snapshot
// operation left anything for recoverLinuxSnapshots to finish.
func linuxSnapshotRecoveryPending(dir string) bool {
	if _, err := os.Lstat(filepath.Join(dir, checkpointRollbackFile)); err == nil {
		return true
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".snapshot-preparing-*"))
	return len(matches) > 0
}

// linuxRollbackKept lists the previous states kept by finished roll backs,
// oldest first. Each is a complete VM folder in the stage's data folder.
func linuxRollbackKept(dir string) []string {
	if _, err := os.Lstat(filepath.Join(dir, checkpointRollbackFile)); err == nil {
		// An unfinished journal still owns its stage.
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var kept []string
	for _, entry := range entries {
		if !entry.IsDir() || !linuxRollbackStage.MatchString(entry.Name()) {
			continue
		}
		data := filepath.Join(dir, entry.Name(), "data")
		if contents, err := os.ReadDir(data); err == nil && len(contents) > 0 {
			kept = append(kept, data)
		}
	}
	return kept
}

// removeLinuxRollbackKept deletes the previous states kept by roll backs.
// Only recognized stage folders inside this VM's folder are removed.
func removeLinuxRollbackKept(dir string) error {
	if _, err := os.Lstat(filepath.Join(dir, checkpointRollbackFile)); err == nil {
		return uiError(uiText("snapshots.linux.a_roll_back_is_not_finished_open_try"), nil)
	}
	for _, data := range linuxRollbackKept(dir) {
		stage := filepath.Dir(data)
		if !linuxRollbackStage.MatchString(filepath.Base(stage)) || filepath.Dir(stage) != filepath.Clean(dir) {
			return uiError(uiTextWith("snapshots.linux.unexpected_kept_state_at", map[string]string{"stage": stage}), nil)
		}
		info, err := os.Lstat(stage)
		if err != nil {
			return err
		}
		if err := validateMovePath(stage); err != nil {
			return err
		}
		if err := rejectMoveLink(stage, info); err != nil {
			return err
		}
		if err := os.RemoveAll(stage); err != nil {
			return err
		}
	}
	return nil
}

// removeLinuxSnapshotStore deletes every snapshot of this VM, then the empty
// store. A damaged entry or an unknown file stops it with the rest kept.
func removeLinuxSnapshotStore(dir string) error {
	store := checkpointStore{installation: dir}
	if err := store.Recover(); err != nil {
		return err
	}
	entries, err := store.List()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := store.Delete(entry.ID); err != nil {
			return uiError(uiTextWith("snapshots.linux.message", map[string]string{"name": entry.Name, "error": fmt.Sprint(err)}), err)
		}
	}
	// What remains is the store's lock file. Anything else is not ours.
	remaining, err := os.ReadDir(store.path())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range remaining {
		if entry.Name() != "mutation.lock" || !entry.Type().IsRegular() {
			return uiError(uiTextWith("snapshots.linux.contains_files_try_omarchy_did_not_create_nothing", map[string]string{"path": store.path()}), nil)
		}
	}
	if err := os.Remove(filepath.Join(store.path(), "mutation.lock")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Remove(store.path())
}

// linuxSnapshotsBytes is the space the snapshot store uses on the drive.
func linuxSnapshotsBytes(dir string) (count int, bytes int64) {
	entries, err := (checkpointStore{installation: dir}).List()
	if err != nil {
		return 0, 0
	}
	for _, entry := range entries {
		count++
		bytes += entry.ArchiveBytes
	}
	return count, bytes
}

func linuxSnapshotRows(entries []vmCheckpoint) []linuxRow {
	if len(entries) == 0 {
		return []linuxRow{{Title: uiText("snapshots.linux.no_snapshots_yet"), Detail: uiText("snapshots.linux.create_one_before_trying_something_you_may_want")}}
	}
	rows := make([]linuxRow, 0, len(entries))
	for _, entry := range entries {
		row := linuxRow{Title: entry.Name, Reply: "snapshot:" + entry.ID}
		if entry.Problem != "" {
			row.Detail, row.State = uiTextWith("snapshots.linux.this_snapshot_cannot_be_used", map[string]string{"problem": entry.Problem}), "unavailable"
		} else {
			row.Detail = entry.Created.Local().Format("Jan 2, 2006 at 15:04") + " · " + linuxGB(entry.ArchiveBytes)
		}
		rows = append(rows, row)
	}
	return rows
}

// linuxDefaultSnapshotName is the suggested name, which the person can edit.
func linuxDefaultSnapshotName(now time.Time) string {
	return "Snapshot " + now.Format("Jan 2 15:04")
}

// linuxSnapshotSpaceNote says what one more snapshot costs before it starts.
func linuxSnapshotSpaceNote(dir string) string {
	note := uiText("snapshots.linux.a_snapshot_is_a_compressed_copy_of_this")
	if used := linuxAllocatedBytes(filepath.Join(dir, "vm", "disk.raw")) + linuxTreeBytes(filepath.Join(dir, "guest")); used > 0 {
		note += uiTextWith("snapshots.linux.this_one_can_take_up_to", map[string]string{"used": linuxGB(used)})
	}
	if free := linuxFreeBytes(dir); free >= 0 {
		note += uiTextWith("snapshots.linux.is_free_on_this_drive", map[string]string{"free": linuxGB(free)})
	}
	return note
}

func linuxSnapshotsResult(err error, success string) string {
	if err == nil {
		return success
	}
	if errors.Is(err, errSetupCancelled) {
		return uiText("snapshots.linux.cancelled_your_vm_and_its_other_snapshots_were")
	}
	var space *insufficientSpaceError
	if errors.As(err, &space) {
		return uiTextWith("snapshots.linux.this_needs_free_on_the_drive_is_available", map[string]string{"space_need": formatGiB(space.need), "space_have": formatGiB(space.have)})
	}
	return capitalizeFirst(err.Error()) + "."
}

// runLinuxSnapshotOperation runs a long store operation with progress and a
// working Cancel, the same way backups do.
func runLinuxSnapshotOperation(w *linuxSetupWindow, status, label string, operation func(backupProgress) error) error {
	configureSetupCancellation(false)
	linuxRecoveryActive.Store(true)
	w.update(linuxSetupState{Status: status})
	err := operation(linuxRecoveryProgress(w, label))
	linuxRecoveryActive.Store(false)
	configureSetupCancellation(false)
	return err
}

// showLinuxSnapshots is the Snapshots page under Backup and recovery. It
// returns to the recovery page with what happened.
func showLinuxSnapshots(w *linuxSetupWindow, dir string) string {
	if !completeInstallExists(dir, "disk.raw") {
		return uiText("snapshots.linux.there_is_no_complete_vm_here_to_snapshot")
	}
	store := checkpointStore{installation: dir}
	status := ""
	for {
		entries, err := store.List()
		if err != nil {
			return uiTextWith("snapshots.linux.could_not_read_snapshots", map[string]string{"error": err.Error()})
		}
		page := linuxSetupState{Prompt: "snapshots", Status: status,
			Sections: []linuxSection{{Rows: linuxSnapshotRows(entries)}},
			Actions:  []linuxAction{{Label: uiText("snapshots.create"), Reply: "create", Suggested: true}, {Label: uiText("launcher.linux.back"), Reply: "close"}}}
		if status == "" {
			page.Status = uiText("snapshots.linux.a_snapshot_saves_this_vm_as_it_is")
		}
		answer, err := w.ask(context.Background(), page)
		if err != nil || answer == "close" || answer == "back" {
			return status
		}
		switch {
		case answer == "create":
			status = createLinuxSnapshot(w, dir)
		case strings.HasPrefix(answer, "snapshot:"):
			id := strings.TrimPrefix(answer, "snapshot:")
			var selected *vmCheckpoint
			for i := range entries {
				if entries[i].ID == id {
					selected = &entries[i]
				}
			}
			if selected == nil {
				status = uiText("snapshots.linux.that_snapshot_is_no_longer_here")
				continue
			}
			status = showLinuxSnapshot(w, dir, *selected)
		}
	}
}

func createLinuxSnapshot(w *linuxSetupWindow, dir string) string {
	name := linuxDefaultSnapshotName(time.Now())
	notice := ""
	for {
		answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "snapshot-name", Title: uiText("launcher.linux.create_a_snapshot"), Text: name, Notice: notice,
			Status: uiTextWith("snapshots.linux.name_this_snapshot_so_you_can_find_it", map[string]string{"linux_snapshot_space_note_dir": linuxSnapshotSpaceNote(dir)})})
		if err != nil || answer == "cancel" {
			return ""
		}
		name = strings.TrimSpace(answer)
		if validCheckpointName(name) {
			break
		}
		notice = uiText("snapshots.linux.use_a_name_of_1_to_80_characters")
	}
	var created vmCheckpoint
	err := runLinuxSnapshotOperation(w, uiText("snapshots.linux.creating_snapshot"), uiText("snapshots.linux.saving_snapshot"), func(report backupProgress) error {
		var err error
		created, err = (checkpointStore{installation: dir}).Create(name, report)
		return err
	})
	return linuxSnapshotsResult(err, uiTextWith("snapshots.linux.snapshot_saved", map[string]string{"created_name": created.Name, "created_archive_bytes": linuxGB(created.ArchiveBytes)}))
}

func showLinuxSnapshot(w *linuxSetupWindow, dir string, entry vmCheckpoint) string {
	store := checkpointStore{installation: dir}
	page := linuxSetupState{Prompt: "snapshot", Title: entry.Name}
	if entry.Problem != "" {
		page.Status = uiTextWith("snapshots.linux.this_snapshot_cannot_be_restored_you_can_delete", map[string]string{"problem": entry.Problem})
		page.Actions = []linuxAction{{Label: uiText("snapshots.linux.delete_snapshot"), Reply: "delete", Destructive: true}, {Label: uiText("launcher.linux.back"), Reply: "close"}}
	} else {
		page.Status = "Saved " + entry.Created.Local().Format("Monday, January 2, 2006 at 15:04") + ". Uses " + linuxGB(entry.ArchiveBytes) + " in this VM's folder."
		page.Actions = []linuxAction{
			{Label: uiText("launcher.linux.restore_as_a_copy"), Reply: "restore", Suggested: true},
			{Label: uiText("snapshots.linux.roll_back_to_this_snapshot"), Reply: "rollback", Destructive: true},
			{Label: uiText("snapshots.linux.delete_snapshot"), Reply: "delete"},
			{Label: uiText("launcher.linux.back"), Reply: "close"},
		}
	}
	answer, err := w.ask(context.Background(), page)
	if err != nil {
		return ""
	}
	switch answer {
	case "restore":
		parent, err := w.ask(context.Background(), linuxSetupState{Prompt: "restore-parent", Status: uiTextWith("snapshots.linux.choose_the_folder_that_will_hold_a_new", map[string]string{"name": entry.Name})})
		if err != nil || parent == "cancel" {
			return ""
		}
		if !filepath.IsAbs(parent) {
			return uiText("recovery.linux.choose_an_absolute_destination_folder")
		}
		destination := filepath.Join(parent, "try-omarchy-snapshot-"+time.Now().Format("20060102-150405"))
		err = runLinuxSnapshotOperation(w, uiText("recovery.linux.restoring_a_copy"), uiText("recovery.linux.restoring"), func(report backupProgress) error {
			return store.Restore(entry.ID, destination, report)
		})
		return linuxSnapshotsResult(err, uiTextWith("snapshots.linux.copy_saved_as_in_the_folder_you_chose", map[string]string{"path": filepath.Base(destination)}))
	case "rollback":
		answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: uiTextWith("snapshots.linux.roll_back_to", map[string]string{"name": entry.Name}), Primary: uiText("snapshots.linux.keep_current_state"), Secondary: uiText("snapshots.linux.roll_back"), Destructive: true,
			Status: uiTextWith("snapshots.linux.rollback_detail", map[string]string{"date": entry.Created.Local().Format("Jan 2 at 15:04")})})
		if err != nil || answer != "secondary" {
			return ""
		}
		var kept string
		err = runLinuxSnapshotOperation(w, uiText("snapshots.linux.rolling_back"), uiText("snapshots.linux.restoring_snapshot"), func(report backupProgress) error {
			var err error
			kept, err = store.Rollback(entry.ID, report)
			return err
		})
		if err != nil {
			return linuxSnapshotsResult(err, "")
		}
		logf("snapshots: rolled back to %s; previous state kept at %s", entry.ID, kept)
		return uiTextWith("snapshots.linux.rolled_back_to_launch_omarchy_to_use_it", map[string]string{"name": entry.Name})
	case "delete":
		answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: "Delete \"" + entry.Name + "\"?", Primary: uiText("snapshots.linux.keep_snapshot"), Secondary: uiText("snapshots.linux.delete_snapshot_2"), Destructive: true,
			Status: uiTextWith("snapshots.linux.this_frees_the_vm_itself_and_its_other", map[string]string{"entry_archive_bytes": linuxGB(entry.ArchiveBytes)})})
		if err != nil || answer != "secondary" {
			return ""
		}
		if err := store.Delete(entry.ID); err != nil {
			return uiTextWith("snapshots.linux.could_not_delete_the_snapshot", map[string]string{"error": err.Error()})
		}
		return uiTextWith("snapshots.linux.snapshot_deleted", map[string]string{"name": entry.Name})
	}
	return ""
}

// cleanupLinuxRollbackKept removes previous states kept by roll backs.
func cleanupLinuxRollbackKept(w *linuxSetupWindow, dir string) string {
	kept := linuxRollbackKept(dir)
	if len(kept) == 0 {
		return uiText("snapshots.linux.there_is_no_state_kept_from_a_roll")
	}
	var size int64
	for _, data := range kept {
		size += linuxTreeBytes(data)
	}
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: uiText("snapshots.linux.remove_the_state_kept_from_roll_back"), Primary: uiText("recovery.linux.keep_it"), Secondary: uiText("snapshots.linux.remove_kept_state"), Destructive: true,
		Status: uiTextWith("snapshots.linux.permanently_remove_the_vm_state_set_aside_by", map[string]string{"size": linuxGB(size), "names": strings.Join(kept, "\n")})})
	if err != nil || answer != "secondary" {
		return uiText("snapshots.linux.the_kept_state_was_not_removed")
	}
	if err := removeLinuxRollbackKept(dir); err != nil {
		return uiTextWith("snapshots.linux.could_not_remove_the_kept_state", map[string]string{"error": err.Error()})
	}
	return uiText("snapshots.linux.the_state_kept_from_the_roll_back_was")
}
