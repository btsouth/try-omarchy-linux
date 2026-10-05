//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// SDL reports host paths. Flatpak does not grant a sandbox access to a path
// merely because its name arrived in a drop event. The file chooser portal
// grants only the files the user explicitly reselects.
func linuxGrantDroppedFiles(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, errors.New(uiText("drop.linux.no_files_were_dropped"))
	}
	accessible := true
	checked := make([]string, 0, len(paths))
	for _, path := range paths {
		canonical, err := linuxTransferSource(path)
		if err != nil {
			accessible = false
			break
		}
		checked = append(checked, canonical)
	}
	if accessible {
		return checked, nil
	}
	w := startLinuxWindow(func() {})
	if w == nil {
		return nil, errors.New(uiText("drop.linux.open_the_app_to_grant_access_to_these"))
	}
	defer w.stop()
	var names []string
	for _, path := range paths {
		names = append(names, filepath.Base(path))
	}
	message := uiTextWith("drop.linux.choose_the_dropped_files_to_give_try_omarchy", map[string]string{"names": strings.Join(names, ", ")})
	value, err := w.ask(context.Background(), linuxSetupState{Prompt: "grant-files", Status: message})
	if err != nil || value == "cancel" {
		return nil, errSetupCancelled
	}
	var granted []string
	if err := json.Unmarshal([]byte(value), &granted); err != nil || len(granted) == 0 || len(granted) > 1000 {
		return nil, errors.New(uiText("drop.linux.the_file_chooser_did_not_grant_any_files"))
	}
	for i, path := range granted {
		canonical, err := linuxTransferSource(path)
		if err != nil {
			return nil, uiError(uiTextWith("drop.linux.cannot_read_chosen_file", map[string]string{"path": fmt.Sprintf("%q", filepath.Base(path)), "error": fmt.Sprint(err)}), err)
		}
		granted[i] = canonical
	}
	return granted, nil
}
