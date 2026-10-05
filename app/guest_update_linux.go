//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// A newer app can pin a newer guest image. The image is Omarchy's system files
// (kernel, initial RAM disk and the factory disk); the person's own disk is
// never part of it. The update is fetched beside the working files in
// guest.next and swapped in only when complete, and the swap is undone at the
// next launch if the new guest never reports ready (see recoverLinuxGuestUpdate).
//
// Two rules keep a person able to use what they already have:
//   - An interrupted download keeps its progress. guest.next is reused on the
//     next launch when it was staged for the same release, and thrown away only
//     when the app now pins a different one.
//   - An update that cannot be fetched or does not fit is postponed, with a
//     notice, and the installed guest starts. It is never a reason Omarchy will
//     not open.
const linuxUpdateTargetFile = ".update-target"

func ensureLinuxGuest(cfg *config, release, sumsSHA256 string) error {
	ready, err := installReceiptMatches(cfg.guestDir, release, sumsSHA256, installedGuestArtifacts)
	if err != nil {
		return fmt.Errorf("reading verified install state: %w", err)
	}
	if ready {
		os.Remove(filepath.Join(cfg.guestDir, "rootfs.ext4.zst"))
		return nil
	}
	oldRelease, oldManifest, haveOld := installReceiptIdentity(cfg.guestDir)
	isUpdate := haveOld && (!releaseLocationsEquivalent(oldRelease, release) || oldManifest != normalizedSHA256(sumsSHA256))
	if !isUpdate {
		return ensureGuestFiles(cfg, release, sumsSHA256)
	}
	err = stageLinuxGuestUpdate(cfg, release, sumsSHA256)
	if err == nil || errors.Is(err, errSetupCancelled) {
		return err
	}
	if intact, checkErr := installReceiptMatches(cfg.guestDir, oldRelease, oldManifest, installedGuestArtifacts); checkErr != nil || !intact {
		return err
	}
	getUI().setUpdating(false)
	postponeLinuxGuestUpdate(err, cfg.dir)
	return nil
}

func stageLinuxGuestUpdate(cfg *config, release, sumsSHA256 string) error {
	ui := getUI()
	ui.setUpdating(true)
	ui.setCatalogStatus("status.preparing_image_update", nil)
	staged := filepath.Join(cfg.dir, "guest.next")
	marker := filepath.Join(staged, linuxUpdateTargetFile)
	target := normalizedRelease(release) + "\n" + normalizedSHA256(sumsSHA256) + "\n"
	if data, err := os.ReadFile(marker); err != nil || string(data) != target {
		// Nothing usable was staged for this release. Start clean.
		if err := os.RemoveAll(staged); err != nil {
			return err
		}
		if err := os.MkdirAll(staged, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(marker, []byte(target), 0o644); err != nil {
			return err
		}
	}
	stagedCfg := *cfg
	stagedCfg.guestDir = staged
	if err := ensureGuestFiles(&stagedCfg, release, sumsSHA256); err != nil {
		return err
	}
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := recordPayloadUpdate(cfg.dir, releaseVersion(release), true, false); err != nil {
		return fmt.Errorf("recording image rollback state: %w", err)
	}
	if err := publishDirectoryUpdate(cfg.guestDir, staged, filepath.Join(cfg.dir, "guest.previous")); err != nil {
		// Keep the rollback record. Publication may have moved the old tree
		// before failing, and the next launch is the safest place to reconcile it.
		return fmt.Errorf("publishing image update: %w", err)
	}
	return nil
}

// postponeLinuxGuestUpdate says why the update did not happen and that the
// installed Omarchy is starting instead.
func postponeLinuxGuestUpdate(err error, dir string) {
	var space *insufficientSpaceError
	reason := uiText("update.linux.prepare_failed")
	switch f := classifyLinuxSetupFailure(err, dir); {
	case errors.As(err, &space):
		reason = uiTextWith("update.linux.space_needed", map[string]string{"size": linuxGB(space.need)})
	case f.Help == "space":
		reason = uiText("update.linux.no_space")
	case f.Help == "downloads" && f.Title == uiText("error.linux.could_not_download_omarchy"):
		reason = uiText("update.linux.download_failed")
	}
	logf("guest update postponed: %v", err)
	tellLinuxUser("guest-update", uiText("update.linux.postponed"), uiTextWith("update.linux.postponed_detail", map[string]string{"reason": reason}))
}
