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
		return "There is no complete VM here to move."
	}
	if retained, _ := linuxRetainedMove(defaultDir, dir); retained != nil {
		return "Remove the previous copy from the last move before moving again."
	}
	parent, err := w.ask(context.Background(), linuxSetupState{Prompt: "move-folder", Status: "Choose the folder that will hold this VM. A try-omarchy folder is created inside it. The current copy is kept until the moved VM starts."})
	if err != nil || parent == "cancel" {
		return ""
	}
	if !filepath.IsAbs(parent) {
		return "Choose an absolute destination folder."
	}
	destination := linuxMoveDestination(parent)
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: "Move this VM?", Primary: "Move this VM", Secondary: "Keep it here",
		Status: "From: " + dir + "\nTo: " + destination + "\n\nFiles are copied and checked before Try Omarchy switches to the new location. The current copy stays until you start the moved VM and remove it from Backup and recovery."})
	if err != nil || answer != "primary" {
		return ""
	}
	s := linuxMoveStore(defaultDir)
	lock, err := lockMoveStore(s)
	if err != nil {
		return "Could not start the move: " + err.Error()
	}
	defer lock.Close()
	configureSetupCancellation(false)
	linuxRecoveryActive.Store(true)
	w.update(linuxSetupState{Status: "Checking the VM and the space needed..."})
	_, err = s.prepare(dir, destination, linuxRecoveryProgress(w, "Moving"))
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
	w.update(linuxSetupState{Status: "Finishing the move. Keep this window open...", NonCancellable: true})
	if err := s.recover(activateLinuxMove); err != nil {
		return "The checked copy is safe. Open Try Omarchy again to finish switching locations: " + err.Error()
	}
	return "This VM now lives at " + destination + ". Launch it to check your files. After it starts, Backup and recovery can remove the previous copy at " + dir + "."
}

func cleanupLinuxMove(w *linuxSetupWindow, defaultDir, dir string) string {
	retained, booted := linuxRetainedMove(defaultDir, dir)
	if retained == nil {
		return "There is no previous copy to remove."
	}
	if !booted {
		return "Start the moved VM once before removing its previous copy."
	}
	answer, err := w.ask(context.Background(), linuxSetupState{Prompt: "choice", Title: "Remove the previous copy?", Primary: "Keep previous copy", Secondary: "Remove previous copy", Destructive: true,
		Status: "Remove the copy this VM was moved from?\n\n" + retained.Source + "\n\nThe moved VM at " + dir + " stays. If files in the previous copy changed since the move, nothing is removed."})
	if err != nil || answer != "secondary" {
		return "The previous copy was kept."
	}
	s := linuxMoveStore(defaultDir)
	lock, err := lockMoveStore(s)
	if err != nil {
		return "Could not remove the previous copy: " + err.Error()
	}
	defer lock.Close()
	w.update(linuxSetupState{Status: "Checking and removing the previous copy. Keep this window open...", NonCancellable: true})
	if err := s.cleanup(dir); err != nil {
		return "Could not remove the previous copy: " + err.Error()
	}
	return "The previous copy was removed. This VM stays at " + dir + "."
}
