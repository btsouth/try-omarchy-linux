//go:build linux

package main

import "fmt"

func configureLinuxSharing(cfg *config, saved *settings, explicit, force bool, choose func(string) (string, error)) error {
	if explicit || choose == nil || (!force && saved.SharedFolderPrompted) {
		return nil
	}
	status := ""
	for {
		answer, err := choose(status)
		if err != nil {
			return err
		}
		path := ""
		if answer != "skip" {
			path, err = validateLinuxSharedFolder(answer, cfg.dir)
			if err != nil {
				status = fmt.Sprintf("Cannot share that folder: %v", err)
				continue
			}
		}
		next := *saved
		next.Share, next.ShareDisabled, next.SharedFolderPrompted = path, false, true
		if err := saveSettings(settingsPath(cfg.dir), next); err != nil {
			return err
		}
		*saved, cfg.share = next, path
		return nil
	}
}
