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
		return nil, errors.New("no files were dropped")
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
		return nil, errors.New("open the app to grant access to these files")
	}
	defer w.stop()
	var names []string
	for _, path := range paths {
		names = append(names, filepath.Base(path))
	}
	message := fmt.Sprintf("Choose the dropped files to give Try Omarchy access. The file picker can grant files outside the app sandbox.\n\nDropped: %s", strings.Join(names, ", "))
	value, err := w.ask(context.Background(), linuxSetupState{Prompt: "grant-files", Status: message})
	if err != nil || value == "cancel" {
		return nil, errSetupCancelled
	}
	var granted []string
	if err := json.Unmarshal([]byte(value), &granted); err != nil || len(granted) == 0 || len(granted) > 1000 {
		return nil, errors.New("the file chooser did not grant any files")
	}
	for i, path := range granted {
		canonical, err := linuxTransferSource(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read chosen file %q: %w", filepath.Base(path), err)
		}
		granted[i] = canonical
	}
	return granted, nil
}
