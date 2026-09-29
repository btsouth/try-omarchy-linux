package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Backups and restores stage large files in a folder the person picked, which
// no part of the app owns. If the launcher is killed mid-way those files stay.
// stagingBegin is told each path before anything is created there, and
// stagingEnd once it is gone, so a launcher that keeps a record can later tell
// what is its own leftover without searching folders. Both stay nil elsewhere.
var (
	stagingBegin func(kind, path string) error
	stagingEnd   func(path string)
)

func newStagingPath(parent, prefix string) (string, error) {
	var token [8]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return filepath.Join(parent, prefix+hex.EncodeToString(token[:])), nil
}

func beginStaging(kind, path string) error {
	if stagingBegin == nil {
		return nil
	}
	return stagingBegin(kind, path)
}

// endStaging reports a finished path, but only once it is really gone. A
// leftover that could not be removed must stay on record.
func endStaging(path string) {
	if stagingEnd == nil {
		return
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		stagingEnd(path)
	}
}

func createStagingDir(parent, prefix, kind string) (string, error) {
	for attempt := 0; attempt < 10; attempt++ {
		path, err := newStagingPath(parent, prefix)
		if err != nil {
			return "", err
		}
		if err := beginStaging(kind, path); err != nil {
			return "", err
		}
		err = os.Mkdir(path, 0o700)
		if err == nil {
			return path, nil
		}
		endStaging(path)
		if !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("could not choose a staging folder name in %s", parent)
}

func createStagingFile(parent, prefix, kind string) (*os.File, error) {
	for attempt := 0; attempt < 10; attempt++ {
		path, err := newStagingPath(parent, prefix)
		if err != nil {
			return nil, err
		}
		if err := beginStaging(kind, path); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return f, nil
		}
		endStaging(path)
		if !os.IsExist(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("could not choose a staging file name in %s", parent)
}

func removeStagingDir(path string) {
	os.RemoveAll(path)
	endStaging(path)
}

func removeStagingFile(path string) {
	os.Remove(path)
	endStaging(path)
}
