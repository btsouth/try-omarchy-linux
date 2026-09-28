//go:build linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// A chosen storage folder is saved as the path the sandbox sees, which for a
// picked folder is an opaque document portal path. Recording where the folder
// really is beside it lets the home say so when that grant is gone, for
// example after the app was reinstalled without its data or while a drive is
// disconnected. It lives with the app's other host records, never in the
// location file itself, which older launchers read strictly.
const linuxLocationHintName = "location-hint.json"

type linuxLocationHint struct {
	Path string `json:"path"`
	Host string `json:"host"`
}

func linuxLocationHintPath(defaultDir string) string {
	return filepath.Join(linuxMoveStore(defaultDir).dir, linuxLocationHintName)
}

// noteLinuxLocationHint records the real path of selected. A path that is
// already a host path needs no record, and an old one is dropped.
func noteLinuxLocationHint(defaultDir, selected string) {
	path := linuxLocationHintPath(defaultDir)
	host := linuxHostPath(selected)
	if host == selected {
		os.Remove(path)
		return
	}
	data, err := json.Marshal(linuxLocationHint{Path: selected, Host: host})
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o700)
	}
	if err == nil {
		staged := path + ".part"
		if err = os.WriteFile(staged, append(data, '\n'), 0o600); err == nil {
			err = os.Rename(staged, path)
		}
	}
	if err != nil {
		logf("storage: could not record where the VM folder is: %v", err)
	}
}

// linuxLocationHostPath names path for a person: the recorded real path when
// the pointer still matches it, otherwise whatever the portal says now.
func linuxLocationHostPath(defaultDir, path string) string {
	data, err := os.ReadFile(linuxLocationHintPath(defaultDir))
	if err == nil && len(data) <= 4096 {
		var hint linuxLocationHint
		if json.Unmarshal(data, &hint) == nil && hint.Path == path && filepath.IsAbs(hint.Host) {
			return linuxShortenHome(hint.Host)
		}
	}
	return linuxDisplayPath(path)
}
