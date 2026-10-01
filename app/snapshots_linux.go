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
		return fmt.Errorf("finishing an interrupted roll back: %w", err)
	}
	if err := (checkpointStore{installation: dir}).Recover(); err != nil {
		return fmt.Errorf("clearing an interrupted snapshot: %w", err)
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
		return fmt.Errorf("a roll back is not finished; open Try Omarchy again before removing its kept state")
	}
	for _, data := range linuxRollbackKept(dir) {
		stage := filepath.Dir(data)
		if !linuxRollbackStage.MatchString(filepath.Base(stage)) || filepath.Dir(stage) != filepath.Clean(dir) {
			return fmt.Errorf("unexpected kept state at %s", stage)
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
			return fmt.Errorf("%s: %w", entry.Name, err)
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
			return fmt.Errorf("%s contains files Try Omarchy did not create; nothing else was removed", store.path())
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
		return []linuxRow{{Title: "No snapshots yet", Detail: "Create one before trying something you may want to undo, such as a big update or a new setup."}}
	}
	rows := make([]linuxRow, 0, len(entries))
	for _, entry := range entries {
		row := linuxRow{Title: entry.Name, Reply: "snapshot:" + entry.ID}
		if entry.Problem != "" {
			row.Detail, row.State = "This snapshot cannot be used: "+entry.Problem, "unavailable"
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
	note := "A snapshot is a compressed copy of this VM, kept in its folder."
	if used := linuxAllocatedBytes(filepath.Join(dir, "vm", "disk.raw")) + linuxTreeBytes(filepath.Join(dir, "guest")); used > 0 {
		note += " This one can take up to " + linuxGB(used) + "."
	}
	if free := linuxFreeBytes(dir); free >= 0 {
		note += " " + linuxGB(free) + " is free on this drive."
	}
	return note
}

func linuxSnapshotsResult(err error, success string) string {
	if err == nil {
		return success
	}
	if errors.Is(err, errSetupCancelled) {
		return "Cancelled. Your VM and its other snapshots were not changed."
	}
	var space *insufficientSpaceError
	if errors.As(err, &space) {
		return fmt.Sprintf("This needs %s free on the drive; %s is available. Free some space, or remove an old snapshot, then try again. Your VM was not changed.", formatGiB(space.need), formatGiB(space.have))
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
		return "There is no complete VM here to snapshot."
	}
	store := checkpointStore{installation: dir}
	status := ""
	for {
		entries, err := store.List()
		if err != nil {
			return "Could not read snapshots: " + err.Error()
		}
		page := linuxSetupState{Prompt: "snapshots", Status: status,
			Sections: []linuxSection{{Heading: "Snapshots", Rows: linuxSnapshotRows(entries)}},
			Actions:  []linuxAction{{Label: "Create snapshot", Reply: "create", Suggested: true}, {Label: "Back", Reply: "close"}}}
		if status == "" {
			page.Status = "A snapshot saves this VM as it is now, so you can return to that point later. Roll back replaces the VM with a snapshot; Restore as a copy makes a separate VM from it."
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
				status = "That snapshot is no longer here."
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
		answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "snapshot-name", Title: "Create a snapshot", Text: name, Notice: notice,
			Status: "Name this snapshot so you can find it later. Shut down Omarchy first if it is running.\n\n" + linuxSnapshotSpaceNote(dir)})
		if err != nil || answer == "cancel" {
			return ""
		}
		name = strings.TrimSpace(answer)
		if validCheckpointName(name) {
			break
		}
		notice = "Use a name of 1 to 80 characters, without line breaks."
	}
	var created vmCheckpoint
	err := runLinuxSnapshotOperation(w, "Creating snapshot...", "Saving snapshot", func(report backupProgress) error {
		var err error
		created, err = (checkpointStore{installation: dir}).Create(name, report)
		return err
	})
	return linuxSnapshotsResult(err, "Snapshot \""+created.Name+"\" saved ("+linuxGB(created.ArchiveBytes)+").")
}

func showLinuxSnapshot(w *linuxSetupWindow, dir string, entry vmCheckpoint) string {
	store := checkpointStore{installation: dir}
	page := linuxSetupState{Prompt: "snapshot", Title: entry.Name}
	if entry.Problem != "" {
		page.Status = "This snapshot cannot be restored: " + entry.Problem + ". You can delete it to free its space."
		page.Actions = []linuxAction{{Label: "Delete snapshot...", Reply: "delete", Destructive: true}, {Label: "Back", Reply: "close"}}
	} else {
		page.Status = "Saved " + entry.Created.Local().Format("Monday, January 2, 2006 at 15:04") + ". Uses " + linuxGB(entry.ArchiveBytes) + " in this VM's folder."
		page.Actions = []linuxAction{
			{Label: "Restore as a copy", Reply: "restore", Suggested: true},
			{Label: "Roll back to this snapshot...", Reply: "rollback", Destructive: true},
			{Label: "Delete snapshot...", Reply: "delete"},
			{Label: "Back", Reply: "close"},
		}
	}
	answer, err := w.ask(context.Background(), page)
	if err != nil {
		return ""
	}
	switch answer {
	case "restore":
		parent, err := w.ask(context.Background(), linuxSetupState{Prompt: "restore-parent", Status: "Choose the folder that will hold a new VM made from \"" + entry.Name + "\". This VM stays as it is."})
		if err != nil || parent == "cancel" {
			return ""
		}
		if !filepath.IsAbs(parent) {
			return "Choose an absolute destination folder."
		}
		destination := filepath.Join(parent, "try-omarchy-snapshot-"+time.Now().Format("20060102-150405"))
		err = runLinuxSnapshotOperation(w, "Restoring a copy...", "Restoring", func(report backupProgress) error {
			return store.Restore(entry.ID, destination, report)
		})
		return linuxSnapshotsResult(err, "Copy saved as "+filepath.Base(destination)+" in the folder you chose. On the home screen, choose Use existing data folder to open it. This VM was not changed.")
	case "rollback":
		answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: "Roll back to \"" + entry.Name + "\"?", Primary: "Keep current state", Secondary: "Roll back", Destructive: true,
			Status: "This replaces this VM's disk, Omarchy's system files and Try Omarchy's settings for it with the snapshot from " + entry.Created.Local().Format("Jan 2 at 15:04") + ". Everything changed inside Omarchy since then is set aside.\n\n" +
				"The current state is kept in this VM's folder until you remove it from Backup and recovery, so nothing is lost yet. Shared folders and backups are not touched. Shut down Omarchy first if it is running."})
		if err != nil || answer != "secondary" {
			return ""
		}
		var kept string
		err = runLinuxSnapshotOperation(w, "Rolling back...", "Restoring snapshot", func(report backupProgress) error {
			var err error
			kept, err = store.Rollback(entry.ID, report)
			return err
		})
		if err != nil {
			return linuxSnapshotsResult(err, "")
		}
		logf("snapshots: rolled back to %s; previous state kept at %s", entry.ID, kept)
		return "Rolled back to \"" + entry.Name + "\". Launch Omarchy to use it. The previous state is kept in this VM's folder until you remove it from Backup and recovery."
	case "delete":
		answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: "Delete \"" + entry.Name + "\"?", Primary: "Keep snapshot", Secondary: "Delete snapshot", Destructive: true,
			Status: "This frees " + linuxGB(entry.ArchiveBytes) + ". The VM itself and its other snapshots are not changed."})
		if err != nil || answer != "secondary" {
			return ""
		}
		if err := store.Delete(entry.ID); err != nil {
			return "Could not delete the snapshot: " + err.Error()
		}
		return "Snapshot \"" + entry.Name + "\" deleted."
	}
	return ""
}

// cleanupLinuxRollbackKept removes previous states kept by roll backs.
func cleanupLinuxRollbackKept(w *linuxSetupWindow, dir string) string {
	kept := linuxRollbackKept(dir)
	if len(kept) == 0 {
		return "There is no state kept from a roll back."
	}
	var size int64
	for _, data := range kept {
		size += linuxTreeBytes(data)
	}
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: "Remove the state kept from roll back?", Primary: "Keep it", Secondary: "Remove kept state", Destructive: true,
		Status: "Permanently remove the VM state set aside by rolling back to a snapshot? This frees about " + linuxGB(size) + ". Files inside it will be lost. The current VM and its snapshots are not affected.\n\n" + strings.Join(kept, "\n")})
	if err != nil || answer != "secondary" {
		return "The kept state was not removed."
	}
	if err := removeLinuxRollbackKept(dir); err != nil {
		return "Could not remove the kept state: " + err.Error()
	}
	return "The state kept from the roll back was removed."
}
