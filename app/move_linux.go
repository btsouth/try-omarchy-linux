//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Move state lives beside the default data folder, in the app's own data
// directory, so it stays put while either installation moves.
func linuxMoveStore(defaultDir string) moveStore {
	return moveStore{dir: filepath.Join(filepath.Dir(defaultDir), "try-omarchy-host"), defaultDir: defaultDir}
}

// The shared move journal already repoints data-location.json when the moved
// installation is the selected one, and Linux has no shortcuts to rewrite.
func activateLinuxMove(*installationMove) error { return nil }

// Called after the launcher owns the lifecycle port. A verified move is
// finished forward; an interrupted copy only discards its staging folder.
func recoverLinuxMove(defaultDir string) error {
	s := linuxMoveStore(defaultDir)
	if _, err := os.Stat(filepath.Join(s.dir, moveStateName)); os.IsNotExist(err) {
		return nil
	}
	lock, err := lockMoveStore(s)
	if err != nil {
		return err
	}
	defer lock.Close()
	return s.recover(activateLinuxMove)
}

// An explicit -dir naming a moved installation opens its new location, so the
// retained original cannot silently diverge from the copy.
func resolveLinuxMovedDirectory(defaultDir, dir string) (string, error) {
	s := linuxMoveStore(defaultDir)
	if _, err := os.Stat(filepath.Join(s.dir, moveStateName)); os.IsNotExist(err) {
		return dir, nil
	}
	state, err := s.load()
	if err != nil {
		return "", err
	}
	return resolveMovedDirectory(state, dir)
}

func markLinuxMovedGuestReady(defaultDir, dir string) {
	s := linuxMoveStore(defaultDir)
	if _, err := os.Stat(filepath.Join(s.dir, moveStateName)); os.IsNotExist(err) {
		return
	}
	lock, err := lockMoveStore(s)
	if err != nil {
		logf("recording moved guest readiness: %v", err)
		return
	}
	defer lock.Close()
	if err := s.markBooted(dir); err != nil {
		logf("recording moved guest readiness: %v", err)
	}
}

// linuxRetainedMove reports the original kept by a move to dir, and whether
// the moved copy has booted so the original may be removed.
func linuxRetainedMove(defaultDir, dir string) (*installationMove, bool) {
	s := linuxMoveStore(defaultDir)
	if _, err := os.Stat(filepath.Join(s.dir, moveStateName)); os.IsNotExist(err) {
		return nil, false
	}
	state, err := s.load()
	if err != nil || state.Retained == nil || !pathsEqual(state.Retained.Destination, dir) {
		return nil, false
	}
	return state.Retained, state.Retained.Booted
}

func linuxMoveDestination(parent string) string {
	if strings.EqualFold(filepath.Base(parent), "try-omarchy") {
		return parent
	}
	return filepath.Join(parent, "try-omarchy")
}

func moveLinuxInstallation(w *linuxSetupWindow, defaultDir, dir string) string {
	if !completeInstallExists(dir, "disk.raw") {
		return uiText("move.linux.there_is_no_complete_vm_here_to_move")
	}
	if retained, _ := linuxRetainedMove(defaultDir, dir); retained != nil {
		return uiText("move.linux.remove_the_previous_copy_from_the_last_move")
	}
	parent, err := w.ask(context.Background(), linuxSetupState{Prompt: "move-folder", Status: uiText("move.linux.choose_the_folder_that_will_hold_this_vm")})
	if err != nil || parent == "cancel" {
		return ""
	}
	if !filepath.IsAbs(parent) {
		return uiText("recovery.linux.choose_an_absolute_destination_folder")
	}
	destination := linuxMoveDestination(parent)
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: uiText("move.linux.move_this_vm"), Primary: uiText("launcher.linux.move_this_vm_2"), Secondary: uiText("move.linux.keep_it_here"),
		Status: uiTextWith("move.linux.from_to_files_are_copied_and_checked_before", map[string]string{"path": dir, "path_2": destination})})
	if err != nil || answer != "primary" {
		return ""
	}
	s := linuxMoveStore(defaultDir)
	lock, err := lockMoveStore(s)
	if err != nil {
		return uiTextWith("move.linux.could_not_start_the_move", map[string]string{"error": err.Error()})
	}
	defer lock.Close()
	configureSetupCancellation(false)
	linuxRecoveryActive.Store(true)
	w.update(linuxSetupState{Status: uiText("move.linux.checking_the_vm_and_the_space_needed")})
	_, err = s.prepare(dir, destination, linuxRecoveryProgress(w, uiText("move.linux.moving")))
	linuxRecoveryActive.Store(false)
	configureSetupCancellation(false)
	if err != nil {
		// Only an unverified copy is discarded here. A verified one is
		// finished forward below or by the next launch.
		if current, e := s.load(); e == nil && current.Pending != nil && current.Pending.Phase == "copying" {
			if e = s.recover(activateLinuxMove); e != nil {
				logf("move staging recovery: %v", e)
			}
		}
		return linuxRecoveryResult(err, "")
	}
	w.update(linuxSetupState{Status: uiText("move.linux.finishing_the_move_keep_this_window_open"), NonCancellable: true})
	if err := s.recover(activateLinuxMove); err != nil {
		return uiTextWith("move.linux.the_checked_copy_is_safe_open_try_omarchy", map[string]string{"error": err.Error()})
	}
	return uiTextWith("move.linux.this_vm_now_lives_at_launch_it_to", map[string]string{"path": destination, "path_2": dir})
}

func cleanupLinuxMove(w *linuxSetupWindow, defaultDir, dir string) string {
	retained, booted := linuxRetainedMove(defaultDir, dir)
	if retained == nil {
		return uiText("move.linux.there_is_no_previous_copy_to_remove")
	}
	if !booted {
		return uiText("move.linux.start_the_moved_vm_once_before_removing_its")
	}
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: uiText("move.linux.remove_the_previous_copy"), Primary: uiText("move.linux.keep_previous_copy"), Secondary: uiText("move.linux.remove_previous_copy"), Destructive: true,
		Status: uiTextWith("move.linux.remove_the_copy_this_vm_was_moved_from", map[string]string{"retained_source": retained.Source, "path": dir})})
	if err != nil || answer != "secondary" {
		return uiText("move.linux.the_previous_copy_was_kept")
	}
	s := linuxMoveStore(defaultDir)
	lock, err := lockMoveStore(s)
	if err != nil {
		return uiTextWith("move.linux.could_not_remove_the_previous_copy", map[string]string{"error": err.Error()})
	}
	defer lock.Close()
	w.update(linuxSetupState{Status: uiText("move.linux.checking_and_removing_the_previous_copy_keep_this"), NonCancellable: true})
	if err := s.cleanup(dir); err != nil {
		return uiTextWith("move.linux.could_not_remove_the_previous_copy", map[string]string{"error": err.Error()})
	}
	return uiTextWith("move.linux.the_previous_copy_was_removed_this_vm_stays", map[string]string{"path": dir})
}
